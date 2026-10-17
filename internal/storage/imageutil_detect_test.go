package storage

import (
	"bytes"
	"encoding/binary"
	"image"
	"testing"
)

// ftypBox builds a minimal ISO BMFF ftyp box with the given major brand and
// compatible brands, e.g. as written by AVIF encoders.
func ftypBox(major string, brands []string) []byte {
	payload := 4 + 4 + len(brands)*4 // major + minor + compatible brands
	box := make([]byte, 8+payload)
	binary.BigEndian.PutUint32(box[0:4], uint32(len(box)))
	copy(box[4:8], "ftyp")
	copy(box[8:12], major)
	binary.BigEndian.PutUint32(box[12:16], 0) // minor version
	for i, b := range brands {
		copy(box[16+i*4:20+i*4], b)
	}
	return box
}

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

	// A real-world AVIF whose ftyp box carries a long compatible-brand list:
	// major brand "isom" (not one of the AVIF majors) with "avif" beyond the
	// 7th brand (offset 44). The old sniff only scanned up to offset 43 and
	// missed it, causing already-AVIF files to be re-encoded.
	longBrands := ftypBox("isom", []string{"isom", "iso2", "mp41", "mif1", "miaf", "heic", "hevc", "avif", "avis"})
	avisMajor := ftypBox("avis", nil)
	heicCompat := ftypBox("heic", []string{"heic", "mif1", "avif"})
	// HEIF generic (HEVC) must NOT be classified as AVIF just because the major
	// brand is "mif1".
	heifOnly := ftypBox("mif1", []string{"mif1"})

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"avif", avifBytes, "avif"},
		{"avif-long-compat-brands", longBrands, "avif"},
		{"avif-avis-major", avisMajor, "avif"},
		{"avif-heic-major-avif-compat", heicCompat, "avif"},
		{"webp", webpBytes, "webp"},
		{"jpeg", jpgBytes, "jpeg"},
		{"png-magic", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}, "png"},
		{"gif-magic", []byte("GIF89a...."), "gif"},
		{"empty", []byte{}, ""},
		{"junk", []byte("hello world this is not an image"), ""},
		{"heif-generic-not-avif", heifOnly, ""},
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
