package storage

import (
	"bytes"
	"image"
	"testing"
)

func TestDetectImageFormat(t *testing.T) {
	// Build a tiny valid AVIF/WebP/JPEG/PNG via the existing encoders.
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))

	avifBytes, _, err := EncodeImage(img, "avif")
	if err != nil {
		t.Fatalf("encode avif: %v", err)
	}
	webpBytes, _, err := EncodeImage(img, "webp")
	if err != nil {
		t.Fatalf("encode webp: %v", err)
	}
	jpgBytes, _, err := EncodeImage(img, "jpeg")
	if err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"avif", avifBytes, "avif"},
		{"webp", webpBytes, "webp"},
		{"jpeg", jpgBytes, "jpeg"},
		{"png-magic", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}, "png"},
		{"gif-magic", []byte("GIF89a...."), "gif"},
		{"empty", []byte{}, ""},
		{"junk", []byte("hello world this is not an image"), ""},
	}
	for _, c := range cases {
		if got := DetectImageFormat(c.data); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}

	// AVIF must start with an ftyp box; assert our sniff actually relies on it.
	if !bytes.HasPrefix(avifBytes[4:8], []byte("ftyp")) {
		t.Errorf("avif bytes do not contain ftyp box at offset 4")
	}
}
