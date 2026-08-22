package storage

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"mujian/internal/config"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/image/draw"
)

// Storage abstracts the cover-file backend (local disk or S3). All cover keys
// are relative storage paths under the "covers/" prefix, named by content
// hash ("covers/<sha256>.<ext>") so identical content is stored once.
type Storage interface {
	// SaveUpload processes an uploaded image (resize + encode), computes its
	// content hash, dedupes against existing covers, writes the file if new,
	// and returns the storage key, the thumbnail storage key, and whether a new
	// file was created.
	SaveUpload(file *multipart.FileHeader) (key, thumb string, created bool, err error)

	// SaveCoverBytes stores raw cover bytes under "covers/<sha256>.<ext>",
	// reusing an existing file when the hash already exists.
	SaveCoverBytes(data []byte, ext string) (key string, created bool, err error)

	// ReadCover returns the bytes for a cover key.
	ReadCover(key string) ([]byte, error)

	// CoverExists reports whether a cover key already exists.
	CoverExists(key string) bool

	// DeleteCover removes a cover key.
	DeleteCover(key string) error

	// MoveCoverToTrash moves a cover key into the trash area (soft delete).
	MoveCoverToTrash(key string) error

	// ListCoverKeys lists all cover keys currently in storage.
	ListCoverKeys() ([]string, error)

	// ListTrashKeys lists all keys currently in the trash area.
	ListTrashKeys() ([]string, error)

	// PurgeTrash permanently deletes all trash contents.
	PurgeTrash() (int, error)

	// ConvertCover reads a cover, re-encodes it to the target format, and
	// writes the new file. Returns the new key and whether the file changed.
	ConvertCover(key string, targetFormat string) (newKey string, converted bool, err error)

	// MakeThumbnail generates a thumbnail of the cover referenced by coverKey
	// from srcData, writes it as an independent file in the given format, and
	// returns the thumbnail storage key (to be stored in records.cover_thumb).
	MakeThumbnail(coverKey string, srcData []byte, maxW int, format string) (string, error)
}

// ---------- shared helpers ----------

func HashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func DetectExt(b []byte) string {
	if len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 {
		return ".jpg"
	}
	if len(b) >= 4 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4e && b[3] == 0x47 {
		return ".png"
	}
	if len(b) >= 12 && b[0] == 'R' && b[1] == 'I' && b[2] == 'F' && b[3] == 'F' && string(b[8:12]) == "WEBP" {
		return ".webp"
	}
	return ".jpg"
}

func decodeImage(b []byte) (image.Image, error) {
	return DecodeImage(b)
}

// ThumbJPEG renders a ≤maxW-wide JPEG thumbnail (quality 60) of img.
func ThumbJPEG(img image.Image, maxW int) ([]byte, error) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w > maxW {
		ratio := float64(maxW) / float64(w)
		dst := image.NewRGBA(image.Rect(0, 0, maxW, int(float64(h)*ratio)))
		catmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}
	var buf bytes.Buffer
	if err := encodeJPEG(&buf, img, 60); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodeThumb scales img down to at most maxW wide and encodes it in the
// requested format (avif/webp/jpeg), returning the raw encoded bytes.
func encodeThumb(img image.Image, maxW int, format string) ([]byte, error) {
	img = ResizeToWidth(img, maxW)
	switch format {
	case "webp":
		b, _, err := EncodeImage(img, "webp")
		return b, err
	case "avif":
		b, _, err := EncodeImage(img, "avif")
		return b, err
	default:
		return ThumbJPEG(img, maxW)
	}
}

// ExtForImageFormat returns the file extension for the given cover format.
func ExtForImageFormat(format string) string {
	switch format {
	case "webp":
		return ".webp"
	case "jpeg":
		return ".jpg"
	default:
		return ".avif"
	}
}

// thumbKeyFor derives the thumbnail storage key for a cover key in the given
// format, e.g. "covers/<hash>.thumb.webp".
func thumbKeyFor(coverKey, format string) string {
	base := strings.TrimSuffix(coverKey, filepath.Ext(coverKey))
	return base + ".thumb" + ExtForImageFormat(format)
}

// isThumbKey reports whether a storage key is a generated thumbnail file.
func isThumbKey(key string) bool {
	return strings.Contains(filepath.Base(key), ".thumb.")
}

var catmullRom = &draw.Kernel{
	Support: 2,
	At: func(t float64) float64 {
		a := 0.5
		if t < 1 {
			return (a+1)*t*t*t - (a+3)*t*t + (a+2)*t
		}
		return a*t*t*t - 5*a*t*t + 8*a*t - 4*a
	},
}

func coverKey(name string) string {
	return "covers/" + name
}

func trashKey(name string) string {
	return "covers_trash/" + name
}

// ---------- Local storage ----------

type LocalStorage struct {
	uploadDir   string
	cfgProvider func() string
}

func NewLocalStorage(uploadDir string, cfgProvider func() string) *LocalStorage {
	os.MkdirAll(uploadDir, 0755)
	os.MkdirAll(filepath.Join(uploadDir, "covers"), 0755)
	os.MkdirAll(filepath.Join(uploadDir, "covers_trash"), 0755)
	if cfgProvider == nil {
		cfgProvider = func() string { return "avif" }
	}
	return &LocalStorage{uploadDir: uploadDir, cfgProvider: cfgProvider}
}

func (s *LocalStorage) imageFormat() string {
	return cmp.Or(s.cfgProvider(), "avif")
}

func (s *LocalStorage) localPath(key string) string {
	clean := filepath.Clean(filepath.Join(s.uploadDir, key))
	root := filepath.Clean(s.uploadDir)
	if !strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return filepath.Join(s.uploadDir, "covers")
	}
	return clean
}

func (s *LocalStorage) SaveUpload(file *multipart.FileHeader) (string, string, bool, error) {
	src, err := file.Open()
	if err != nil {
		return "", "", false, err
	}
	defer src.Close()

	img, _, err := image.Decode(src)
	if err != nil {
		return "", "", false, fmt.Errorf("decode image: %w", err)
	}
	// resize to max poster width
	bounds := img.Bounds()
	if w := bounds.Dx(); w > maxPosterWidth {
		ratio := float64(maxPosterWidth) / float64(w)
		dst := image.NewRGBA(image.Rect(0, 0, maxPosterWidth, int(float64(bounds.Dy())*ratio)))
		catmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	format := s.imageFormat()
	encodedBytes, ext, err := EncodeImage(img, format)
	if err != nil {
		return "", "", false, fmt.Errorf("encode %s: %w", format, err)
	}

	key, created, err := s.SaveCoverBytes(encodedBytes, ext)
	if err != nil {
		return "", "", false, err
	}

	thumbKey, err := s.MakeThumbnail(key, encodedBytes, 400, format)
	if err != nil {
		return "", "", false, fmt.Errorf("thumbnail: %w", err)
	}
	return key, thumbKey, created, nil
}

func (s *LocalStorage) MakeThumbnail(coverKey string, srcData []byte, maxW int, format string) (string, error) {
	img, err := DecodeImage(srcData)
	if err != nil {
		return "", err
	}
	b, err := encodeThumb(img, maxW, format)
	if err != nil {
		return "", err
	}
	return s.saveThumb(coverKey, b, format)
}

func (s *LocalStorage) saveThumb(coverKey string, data []byte, format string) (string, error) {
	key := thumbKeyFor(coverKey, format)
	for _, old := range s.thumbKeysFor(coverKey) {
		if old != key {
			_ = os.Remove(s.localPath(old))
		}
	}
	path := s.localPath(key)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return key, nil
}

// thumbKeysFor returns the thumbnail keys (any format) for a given cover key.
func (s *LocalStorage) thumbKeysFor(coverKey string) []string {
	base := strings.TrimSuffix(coverKey, filepath.Ext(coverKey))
	prefix := filepath.Base(base) + ".thumb."
	dir := filepath.Join(s.uploadDir, "covers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			out = append(out, "covers/"+e.Name())
		}
	}
	return out
}

func (s *LocalStorage) SaveCoverBytes(data []byte, ext string) (string, bool, error) {
	if ext == "" {
		ext = DetectExt(data)
	}
	name := HashBytes(data) + ext
	key := coverKey(name)
	if s.CoverExists(key) {
		return key, false, nil
	}
	path := s.localPath(key)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", false, err
	}
	return key, true, nil
}

func (s *LocalStorage) ReadCover(key string) ([]byte, error) {
	return os.ReadFile(s.localPath(key))
}

func (s *LocalStorage) CoverExists(key string) bool {
	_, err := os.Stat(s.localPath(key))
	return err == nil
}

func (s *LocalStorage) DeleteCover(key string) error {
	for _, t := range s.thumbKeysFor(key) {
		_ = os.Remove(s.localPath(t))
	}
	err := os.Remove(s.localPath(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *LocalStorage) MoveCoverToTrash(key string) error {
	for _, t := range s.thumbKeysFor(key) {
		_ = os.Remove(s.localPath(t))
	}
	from := s.localPath(key)
	to := filepath.Join(s.uploadDir, trashKey(filepath.Base(key)))
	os.MkdirAll(filepath.Dir(to), 0755)
	err := os.Rename(from, to)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *LocalStorage) ListCoverKeys() ([]string, error) {
	keys, err := s.listKeys("covers")
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(keys, isThumbKey), nil
}

func (s *LocalStorage) ListTrashKeys() ([]string, error) {
	return s.listKeys("covers_trash")
}

func (s *LocalStorage) listKeys(sub string) ([]string, error) {
	dir := filepath.Join(s.uploadDir, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		keys = append(keys, sub+"/"+e.Name())
	}
	slices.Sort(keys)
	return keys, nil
}

func (s *LocalStorage) PurgeTrash() (int, error) {
	dir := filepath.Join(s.uploadDir, "covers_trash")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			n++
		}
	}
	return n, nil
}

func (s *LocalStorage) ConvertCover(key string, targetFormat string) (string, bool, error) {
	data, err := s.ReadCover(key)
	if err != nil {
		return "", false, err
	}

	img, err := decodeImage(data)
	if err != nil {
		return "", false, err
	}

	encodedBytes, ext, err := EncodeImage(img, targetFormat)
	if err != nil {
		return "", false, err
	}

	newKey, created, err := s.SaveCoverBytes(encodedBytes, ext)
	if err != nil {
		return "", false, err
	}

	// Delete old file if it's different from the new one
	if newKey != key {
		s.DeleteCover(key)
	}

	return newKey, created, nil
}

// ---------- S3 storage ----------

type S3Storage struct {
	client      *s3.Client
	bucket      string
	publicURL   string
	baseKey     string
	cfgProvider func() string
}

func NewS3Storage(cfg *config.Config) *S3Storage {
	creds := credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")
	client := s3.New(s3.Options{
		Region:       cfg.S3Region,
		BaseEndpoint: aws.String(cfg.S3Endpoint),
		Credentials:  creds,
	})
	return &S3Storage{
		client:      client,
		bucket:      cfg.S3Bucket,
		publicURL:   cfg.S3PublicURL,
		cfgProvider: func() string { return cfg.ImageFormat },
	}
}

func (s *S3Storage) imageFormat() string {
	return cmp.Or(s.cfgProvider(), "avif")
}

func (s *S3Storage) SaveUpload(file *multipart.FileHeader) (string, string, bool, error) {
	src, err := file.Open()
	if err != nil {
		return "", "", false, err
	}
	defer src.Close()

	img, _, err := image.Decode(src)
	if err != nil {
		return "", "", false, fmt.Errorf("decode image: %w", err)
	}
	bounds := img.Bounds()
	if w := bounds.Dx(); w > maxPosterWidth {
		ratio := float64(maxPosterWidth) / float64(w)
		dst := image.NewRGBA(image.Rect(0, 0, maxPosterWidth, int(float64(bounds.Dy())*ratio)))
		catmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}
	format := s.imageFormat()
	encodedBytes, ext, err := EncodeImage(img, format)
	if err != nil {
		return "", "", false, err
	}
	key, created, err := s.SaveCoverBytes(encodedBytes, ext)
	if err != nil {
		return "", "", false, err
	}
	thumbKey, err := s.MakeThumbnail(key, encodedBytes, 400, format)
	if err != nil {
		return "", "", false, err
	}
	return key, thumbKey, created, nil
}

func (s *S3Storage) MakeThumbnail(coverKey string, srcData []byte, maxW int, format string) (string, error) {
	img, err := DecodeImage(srcData)
	if err != nil {
		return "", err
	}
	b, err := encodeThumb(img, maxW, format)
	if err != nil {
		return "", err
	}
	return s.saveThumb(coverKey, b, format)
}

func (s *S3Storage) saveThumb(coverKey string, data []byte, format string) (string, error) {
	key := thumbKeyFor(coverKey, format)
	base := strings.TrimSuffix(coverKey, filepath.Ext(coverKey))
	prefix := "covers/" + filepath.Base(base) + ".thumb."
	if keys, err := s.listKeys(prefix); err == nil {
		for _, k := range keys {
			if k != key {
				_, _ = s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
					Bucket: aws.String(s.bucket), Key: aws.String(k),
				})
			}
		}
	}
	_, err := s.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		CacheControl: aws.String("public, max-age=2592000, immutable"),
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

func (s *S3Storage) SaveCoverBytes(data []byte, ext string) (string, bool, error) {
	if ext == "" {
		ext = DetectExt(data)
	}
	name := HashBytes(data) + ext
	key := coverKey(name)
	if s.CoverExists(key) {
		return key, false, nil
	}
	_, err := s.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		CacheControl: aws.String("public, max-age=2592000, immutable"),
	})
	if err != nil {
		return "", false, fmt.Errorf("s3 put: %w", err)
	}
	return key, true, nil
}

func (s *S3Storage) ReadCover(key string) ([]byte, error) {
	out, err := s.client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *S3Storage) CoverExists(key string) bool {
	_, err := s.client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err == nil
}

func (s *S3Storage) DeleteCover(key string) error {
	base := strings.TrimSuffix(key, filepath.Ext(key))
	prefix := "covers/" + filepath.Base(base) + ".thumb."
	if keys, err := s.listKeys(prefix); err == nil {
		for _, k := range keys {
			_, _ = s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
				Bucket: aws.String(s.bucket), Key: aws.String(k),
			})
		}
	}
	_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *S3Storage) ListCoverKeys() ([]string, error) {
	keys, err := s.listKeys("covers/")
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(keys, isThumbKey), nil
}

func (s *S3Storage) MoveCoverToTrash(key string) error {
	name := filepath.Base(key)
	if _, err := s.client.CopyObject(context.TODO(), &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(s.bucket + "/" + key),
		Key:        aws.String(trashKey(name)),
	}); err != nil {
		return err
	}
	return s.DeleteCover(key)
}

func (s *S3Storage) ListTrashKeys() ([]string, error) {
	return s.listKeys("covers_trash/")
}

func (s *S3Storage) listKeys(prefix string) ([]string, error) {
	var keys []string
	var token *string
	for {
		out, err := s.client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range out.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			break
		}
		token = out.NextContinuationToken
	}
	return keys, nil
}

func (s *S3Storage) PurgeTrash() (int, error) {
	keys, err := s.ListTrashKeys()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, k := range keys {
		if err := s.DeleteCover(k); err == nil {
			n++
		}
	}
	return n, nil
}

func (s *S3Storage) ConvertCover(key string, targetFormat string) (string, bool, error) {
	data, err := s.ReadCover(key)
	if err != nil {
		return "", false, err
	}

	img, err := decodeImage(data)
	if err != nil {
		return "", false, err
	}

	encodedBytes, ext, err := EncodeImage(img, targetFormat)
	if err != nil {
		return "", false, err
	}

	newKey, created, err := s.SaveCoverBytes(encodedBytes, ext)
	if err != nil {
		return "", false, err
	}

	// Delete old file if it's different from the new one
	if newKey != key {
		s.DeleteCover(key)
	}

	return newKey, created, nil
}

// New returns the storage backend selected by the configuration.
func New(cfg *config.Config) Storage {
	if cfg.StorageType == "s3" && cfg.S3Bucket != "" && cfg.S3AccessKey != "" {
		return NewS3Storage(cfg)
	}
	return NewLocalStorage(cfg.UploadDir, func() string { return cfg.ImageFormat })
}
