package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testImage renders a small colored RGBA image.
func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	return img
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newLocal(t *testing.T, format string) *LocalStorage {
	t.Helper()
	dir := t.TempDir()
	s := NewLocalStorage(dir, func() string { return format })
	return s
}

func multipartFileHeader(t *testing.T, field, filename string, data []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	w.Close()

	r := multipart.NewReader(&buf, w.Boundary())
	form, err := r.ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	files := form.File[field]
	if len(files) == 0 {
		t.Fatal("no files in form")
	}
	return files[0]
}

func TestLocalSaveUpload(t *testing.T) {
	s := newLocal(t, "avif")
	key, thumb, created, err := s.SaveUpload(multipartFileHeader(t, "file", "poster.jpg", jpegBytes(t)))
	if err != nil {
		t.Fatalf("SaveUpload: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if !strings.HasPrefix(key, "covers/") || !strings.HasSuffix(key, ".avif") {
		t.Errorf("key shape wrong: %q", key)
	}
	if thumb == "" || !strings.Contains(thumb, ".thumb.") {
		t.Errorf("thumb key shape wrong: %q", thumb)
	}
	if !s.CoverExists(key) {
		t.Error("cover should exist after upload")
	}
	data, err := s.ReadCover(key)
	if err != nil || len(data) == 0 {
		t.Errorf("ReadCover: %v", err)
	}
	// Dedupe: uploading identical bytes reuses the same key.
	key2, _, created2, err := s.SaveUpload(multipartFileHeader(t, "file", "poster2.jpg", jpegBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	if key2 != key || created2 {
		t.Errorf("expected dedupe: key2=%q key=%q created=%v", key2, key, created2)
	}

	// A smaller format (webp) yields a different file.
	s2 := newLocal(t, "webp")
	keyW, _, _, err := s2.SaveUpload(multipartFileHeader(t, "file", "p.jpg", jpegBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(keyW, ".webp") {
		t.Errorf("webp upload should produce .webp, got %q", keyW)
	}
}

func TestSaveCoverBytesAndDetectExt(t *testing.T) {
	s := newLocal(t, "avif")
	jpg := jpegBytes(t)
	key, created, err := s.SaveCoverBytes(jpg, "")
	if err != nil {
		t.Fatal(err)
	}
	if !created || !strings.HasSuffix(key, ".jpg") {
		t.Errorf("jpg bytes should default to .jpg ext: key=%q created=%v", key, created)
	}
	// Same bytes -> dedupe.
	key2, created2, err := s.SaveCoverBytes(jpg, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	if key2 != key || created2 {
		t.Errorf("dedupe failed: %q %v", key2, created2)
	}
	// PNG ext detection.
	pngK, _, err := s.SaveCoverBytes(pngBytes(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(pngK, ".png") {
		t.Errorf("png should be detected: %q", pngK)
	}
}

func TestListCoverKeysFiltersThumbs(t *testing.T) {
	s := newLocal(t, "avif")
	jpg := jpegBytes(t)
	key, _, err := s.SaveCoverBytes(jpg, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MakeThumbnail(key, jpg, 400, "avif"); err != nil {
		t.Fatal(err)
	}
	keys, err := s.ListCoverKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != key {
		t.Errorf("expected exactly the cover (thumbs filtered): %v", keys)
	}
	for _, k := range keys {
		if isThumbKey(k) {
			t.Errorf("thumb key leaked into list: %q", k)
		}
	}
}

func TestConvertCover(t *testing.T) {
	s := newLocal(t, "avif")
	jpg := jpegBytes(t)
	key, _, err := s.SaveCoverBytes(jpg, ".jpg")
	if err != nil {
		t.Fatal(err)
	}

	newKey, converted, err := s.ConvertCover(key, "avif")
	if err != nil {
		t.Fatal(err)
	}
	if !converted || newKey == key {
		t.Errorf("expected a real conversion: newKey=%q old=%q", newKey, key)
	}
	if !strings.HasSuffix(newKey, ".avif") {
		t.Errorf("converted key should be .avif: %q", newKey)
	}
	if s.CoverExists(key) {
		t.Error("old file should be deleted after conversion")
	}
	if !s.CoverExists(newKey) {
		t.Error("new file should exist")
	}

	// Re-encoding the (already avif) file succeeds and stays avif. The avif-go
	// encoder is not byte-deterministic, so the key may differ; the batch
	// conversion never reaches this path because it skips files already in the
	// target format via content sniffing.
	newKey2, _, err := s.ConvertCover(newKey, "avif")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(newKey2, ".avif") {
		t.Errorf("re-encoded key should be .avif: %q", newKey2)
	}
	if !s.CoverExists(newKey2) {
		t.Error("re-encoded file should exist")
	}

	// Corrupt input errors.
	if _, _, err := s.ConvertCover("covers/doesnotexist.jpg", "avif"); err == nil {
		t.Error("converting missing cover should error")
	}
}

func TestTrashFlow(t *testing.T) {
	s := newLocal(t, "avif")
	jpg := jpegBytes(t)
	key, _, err := s.SaveCoverBytes(jpg, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MoveCoverToTrash(key); err != nil {
		t.Fatal(err)
	}
	if s.CoverExists(key) {
		t.Error("cover should be gone after moving to trash")
	}
	trash, err := s.ListTrashKeys()
	if err != nil || len(trash) != 1 {
		t.Errorf("expected 1 trash key: %v err=%v", trash, err)
	}
	// Moving a missing file is not an error.
	if err := s.MoveCoverToTrash("covers/nope.jpg"); err != nil {
		t.Errorf("move missing: %v", err)
	}
	// DeleteCover on missing is not an error.
	if err := s.DeleteCover("covers/nope.jpg"); err != nil {
		t.Errorf("delete missing: %v", err)
	}
	n, err := s.PurgeTrash()
	if err != nil || n != 1 {
		t.Errorf("purge: n=%d err=%v", n, err)
	}
	trash2, _ := s.ListTrashKeys()
	if len(trash2) != 0 {
		t.Errorf("trash should be empty: %v", trash2)
	}
}

func TestMakeThumbnailAndDelete(t *testing.T) {
	s := newLocal(t, "avif")
	jpg := jpegBytes(t)
	key, _, err := s.SaveCoverBytes(jpg, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	thumb, err := s.MakeThumbnail(key, jpg, 400, "jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(thumb, ".thumb.jpg") {
		t.Errorf("thumb key: %q", thumb)
	}
	// Switching format removes the old thumb.
	thumb2, err := s.MakeThumbnail(key, jpg, 200, "webp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(thumb2, ".thumb.webp") {
		t.Errorf("thumb2 key: %q", thumb2)
	}
	if s.CoverExists(thumb) {
		t.Error("old-format thumb should be removed")
	}

	// DeleteCover removes cover + remaining thumbs.
	if err := s.DeleteCover(key); err != nil {
		t.Fatal(err)
	}
	if s.CoverExists(key) || s.CoverExists(thumb2) {
		t.Error("cover and thumbs should be gone after delete")
	}
}

func TestHashBytesAndHelpers(t *testing.T) {
	h1 := HashBytes([]byte("hello"))
	h2 := HashBytes([]byte("hello"))
	h3 := HashBytes([]byte("world"))
	if h1 != h2 || h1 == h3 || len(h1) != 64 {
		t.Errorf("HashBytes wrong: %q %q %q", h1, h2, h3)
	}
	if got := DetectExt(jpegBytes(t)); got != ".jpg" {
		t.Errorf("DetectExt jpeg: %q", got)
	}
	if got := DetectExt(pngBytes(t)); got != ".png" {
		t.Errorf("DetectExt png: %q", got)
	}
	if got := DetectExt([]byte("RIFFxxxxWEBP")); got != ".webp" {
		t.Errorf("DetectExt webp: %q", got)
	}
	if got := DetectExt([]byte("????")); got != ".jpg" {
		t.Errorf("DetectExt unknown should default .jpg: %q", got)
	}
	if got := coverKey("x.jpg"); got != "covers/x.jpg" {
		t.Errorf("coverKey: %q", got)
	}
	if got := trashKey("x.jpg"); got != "covers_trash/x.jpg" {
		t.Errorf("trashKey: %q", got)
	}
	if !isThumbKey("covers/abc.thumb.avif") || isThumbKey("covers/abc.avif") {
		t.Error("isThumbKey wrong")
	}
	if got := thumbKeyFor("covers/abc.jpg", "webp"); got != "covers/abc.thumb.webp" {
		t.Errorf("thumbKeyFor: %q", got)
	}
	if got := extForImageFormat("webp"); got != ".webp" {
		t.Errorf("extForImageFormat webp: %q", got)
	}
}

func TestLocalStorageEdgeCases(t *testing.T) {
	dir := t.TempDir()
	s := NewLocalStorage(dir, nil) // nil provider -> default avif
	if got := s.imageFormat(); got != "avif" {
		t.Errorf("nil provider should default to avif: %q", got)
	}
	// localPath traversal guard.
	p := s.localPath("../../etc/passwd")
	if strings.Contains(p, "..") || filepath.Clean(p) != filepath.Join(dir, "covers") {
		t.Errorf("localPath should clamp to covers dir: %q", p)
	}
	// listKeys on missing dir returns empty.
	keys, err := s.listKeys("missing")
	if err != nil || len(keys) != 0 {
		t.Errorf("listKeys missing dir: %v %v", keys, err)
	}
	// Empty SaveCoverBytes ext with unknown magic -> DetectExt -> .jpg.
	key, created, err := s.SaveCoverBytes([]byte("nonsense-bytes-here"), "")
	if err != nil || !created {
		t.Errorf("SaveCoverBytes unknown: %v %v", key, err)
	}
	// SaveUpload with invalid image errors.
	if _, _, _, err := s.SaveUpload(multipartFileHeader(t, "file", "bad.jpg", []byte("not an image"))); err == nil {
		t.Error("SaveUpload with invalid image should error")
	}
	// MakeThumbnail with undecodable data errors.
	if _, err := s.MakeThumbnail("covers/x.jpg", []byte("garbage"), 400, "avif"); err == nil {
		t.Error("MakeThumbnail with garbage should error")
	}
}

func TestImageFormatProvider(t *testing.T) {
	format := "webp"
	dir := t.TempDir()
	s := NewLocalStorage(dir, func() string { return format })
	if got := s.imageFormat(); got != "webp" {
		t.Errorf("imageFormat: %q", got)
	}
	format = "jpeg"
	if got := s.imageFormat(); got != "jpeg" {
		t.Errorf("imageFormat after change: %q", got)
	}
}

func TestFileLayout(t *testing.T) {
	dir := t.TempDir()
	NewLocalStorage(dir, nil)
	for _, sub := range []string{"covers", "covers_trash"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
			t.Errorf("expected dir %s: %v", sub, err)
		}
	}
}

func TestResizeAndThumbJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 600, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.RGBA{uint8(x / 2), 100, 50, 255})
		}
	}

	// Downscale path.
	small := ResizeToWidth(img, 300)
	if small.Bounds().Dx() != 300 {
		t.Errorf("ResizeToWidth downscale: got width %d, want 300", small.Bounds().Dx())
	}
	// No-upscale path returns the original image.
	if ResizeToWidth(img, 1200) != img {
		t.Error("ResizeToWidth should not upscale")
	}

	// ThumbJPEG with resize.
	b1, err := ThumbJPEG(img, 300)
	if err != nil {
		t.Fatalf("ThumbJPEG resize: %v", err)
	}
	if !bytes.HasPrefix(b1, []byte{0xFF, 0xD8}) {
		t.Error("ThumbJPEG should produce jpeg magic")
	}
	// ThumbJPEG without resize (img already ≤ maxW).
	if _, err := ThumbJPEG(img, 2000); err != nil {
		t.Fatalf("ThumbJPEG no-resize: %v", err)
	}
}

func TestEncodeThumbFormats(t *testing.T) {
	s := newLocal(t, "avif")
	wide := image.NewRGBA(image.Rect(0, 0, 500, 100))
	jpg := jpegBytes(t)
	key, _, err := s.SaveCoverBytes(jpg, "")
	if err != nil {
		t.Fatal(err)
	}
	// MakeThumbnail with a wide source forces the resize branch inside encodeThumb.
	if _, err := s.MakeThumbnail(key, encodeRawPNG(t, wide), 400, "avif"); err != nil {
		t.Fatalf("thumb avif: %v", err)
	}
	if _, err := s.MakeThumbnail(key, encodeRawPNG(t, wide), 400, "webp"); err != nil {
		t.Fatalf("thumb webp: %v", err)
	}
	if _, err := s.MakeThumbnail(key, encodeRawPNG(t, wide), 400, "jpeg"); err != nil {
		t.Fatalf("thumb jpeg: %v", err)
	}
}

func encodeRawPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
