package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/vegidio/avif-go"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// Storage abstracts the cover-file backend (local disk or S3). All cover keys
// are relative storage paths under the "covers/" prefix, named by content
// hash ("covers/<sha256>.<ext>") so identical content is stored once.
type Storage interface {	// SaveUpload processes an uploaded image (resize + AVIF), computes its
	// content hash, dedupes against existing covers, writes the file if new,
	// and returns the storage key, a base64 JPEG thumbnail, and whether a new
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
	img, _, err := image.Decode(bytes.NewReader(b))
	if err == nil {
		return img, nil
	}
	if img, err := webp.Decode(bytes.NewReader(b)); err == nil {
		return img, nil
	}
	return nil, fmt.Errorf("unsupported image format")
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

func ThumbBase64(img image.Image, maxW int) (string, error) {
	b, err := ThumbJPEG(img, maxW)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// ThumbBase64FromBytes decodes jpeg/png/webp bytes and returns a base64 JPEG
// thumbnail (≤maxW wide).
func ThumbBase64FromBytes(data []byte, maxW int) (string, error) {
	img, err := decodeImage(data)
	if err != nil {
		return "", err
	}
	return ThumbBase64(img, maxW)
}

func encodeAVIF(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := avif.Encode(&buf, img, &avif.Options{
		Speed:        6,
		ColorQuality: 65,
		AlphaQuality: 60,
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
	uploadDir string
}

func NewLocalStorage(uploadDir string) *LocalStorage {
	os.MkdirAll(uploadDir, 0755)
	os.MkdirAll(filepath.Join(uploadDir, "covers"), 0755)
	os.MkdirAll(filepath.Join(uploadDir, "covers_trash"), 0755)
	return &LocalStorage{uploadDir: uploadDir}
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

	thumb, err := ThumbBase64(img, 400)
	if err != nil {
		return "", "", false, fmt.Errorf("thumbnail: %w", err)
	}

	avifBytes, err := encodeAVIF(img)
	if err != nil {
		return "", "", false, fmt.Errorf("encode avif: %w", err)
	}

	key, created, err := s.SaveCoverBytes(avifBytes, ".avif")
	if err != nil {
		return "", "", false, err
	}
	return key, thumb, created, nil
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
	err := os.Remove(s.localPath(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *LocalStorage) MoveCoverToTrash(key string) error {
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
	return s.listKeys("covers")
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
	sort.Strings(keys)
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

// ---------- S3 storage ----------

type S3Storage struct {
	client    *s3.Client
	bucket    string
	publicURL string
	baseKey   string
}

func NewS3Storage(cfg *config.Config) *S3Storage {
	creds := credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")
	client := s3.New(s3.Options{
		Region:       cfg.S3Region,
		BaseEndpoint: aws.String(cfg.S3Endpoint),
		Credentials:  creds,
	})
	return &S3Storage{
		client:    client,
		bucket:    cfg.S3Bucket,
		publicURL: cfg.S3PublicURL,
	}
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
	thumb, err := ThumbBase64(img, 400)
	if err != nil {
		return "", "", false, err
	}
	avifBytes, err := encodeAVIF(img)
	if err != nil {
		return "", "", false, err
	}
	key, created, err := s.SaveCoverBytes(avifBytes, ".avif")
	if err != nil {
		return "", "", false, err
	}
	return key, thumb, created, nil
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
	_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
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

func (s *S3Storage) ListCoverKeys() ([]string, error) {
	return s.listKeys("covers/")
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

// New returns the storage backend selected by the configuration.
func New(cfg *config.Config) Storage {
	if cfg.StorageType == "s3" && cfg.S3Bucket != "" && cfg.S3AccessKey != "" {
		return NewS3Storage(cfg)
	}
	return NewLocalStorage(cfg.UploadDir)
}
