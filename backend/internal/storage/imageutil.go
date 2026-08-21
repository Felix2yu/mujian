package storage

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"io"

	"github.com/chai2010/webp"
	"github.com/vegidio/avif-go"
	"golang.org/x/image/draw"
	xwebp "golang.org/x/image/webp"
)

// DetectImageFormat returns the underlying image format of the encoded bytes
// ("avif", "webp", "jpeg", "png", "gif") by sniffing magic bytes. It does not
// rely on the file extension, so a misnamed file (e.g. a real AVIF saved as
// .jpg) is still classified correctly. Returns "" if undetermined.
func DetectImageFormat(data []byte) string {
	// JPEG
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		return "jpeg"
	}
	// PNG
	if len(data) >= 4 && bytes.Equal(data[0:4], []byte{0x89, 0x50, 0x4E, 0x47}) {
		return "png"
	}
	// GIF
	if len(data) >= 4 && bytes.Equal(data[0:4], []byte("GIF8")) {
		return "gif"
	}
	// ISO BMFF / ftyp box (AVIF/HEIF) and WebP need at least 12 bytes.
	if len(data) >= 12 {
		// WebP: "RIFF" .... "WEBP"
		if bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
			return "webp"
		}
		// AVIF/HEIF: ftyp box whose major brand or compatible-brand list carries
		// "avif"/"avis". Only those brands indicate AV1 encoding; "mif1" alone is
		// the generic HEIF brand and must NOT be treated as AVIF (it may be
		// HEVC). Some encoders write long compatible-brand lists, so scan the
		// whole list as bounded by the ftyp box size rather than a fixed window.
		if bytes.Equal(data[4:8], []byte("ftyp")) {
			if major := string(data[8:12]); major == "avif" || major == "avis" {
				return "avif"
			}
			end := int(binary.BigEndian.Uint32(data[0:4]))
			if end < 16 || end > len(data) {
				// Malformed or size-0 box ("extends to EOF"): scan a generous
				// window instead.
				end = len(data)
				if end > 128 {
					end = 128
				}
			}
			for i := 16; i+4 <= end; i += 4 {
				switch string(data[i : i+4]) {
				case "avif", "avis":
					return "avif"
				}
			}
		}
	}
	return ""
}

// DecodeImage decodes jpeg/png/gif/webp/avif bytes into an image.Image.
func DecodeImage(b []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err == nil {
		return img, nil
	}
	if img, err := xwebp.Decode(bytes.NewReader(b)); err == nil {
		return img, nil
	}
	if img, err := avif.Decode(bytes.NewReader(b)); err == nil {
		return img, nil
	}
	return nil, err
}

const maxPosterWidth = 2000

// EncodeImage encodes img to the specified format and returns the bytes and extension.
// Supported formats: "avif", "webp", "jpeg"
func EncodeImage(img image.Image, format string) ([]byte, string, error) {
	switch format {
	case "webp":
		return encodeWebP(img, 80)
	case "jpeg":
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), ".jpg", nil
	default: // "avif"
		return encodeAVIF(img)
	}
}

// encodeJPEG writes img as a JPEG with the given quality.
func encodeJPEG(w io.Writer, img image.Image, quality int) error {
	return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
}

// encodeAVIF encodes img to AVIF format.
func encodeAVIF(img image.Image) ([]byte, string, error) {
	var buf bytes.Buffer
	if err := avif.Encode(&buf, img, &avif.Options{
		Speed:        6,
		ColorQuality: 65,
		AlphaQuality: 60,
	}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), ".avif", nil
}

// encodeWebP encodes img to WebP format.
func encodeWebP(img image.Image, quality float64) ([]byte, string, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, &webp.Options{
		Quality:  float32(quality),
		Lossless: false,
	}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), ".webp", nil
}

// ResizeToWidth scales img down to at most maxW wide (no upscale).
func ResizeToWidth(img image.Image, maxW int) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	if w <= maxW {
		return img
	}
	ratio := float64(maxW) / float64(w)
	dst := image.NewRGBA(image.Rect(0, 0, maxW, int(float64(bounds.Dy())*ratio)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst
}
