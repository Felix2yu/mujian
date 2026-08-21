package storage

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"

	"github.com/chai2010/webp"
	"github.com/vegidio/avif-go"
	"golang.org/x/image/draw"
	xwebp "golang.org/x/image/webp"
)

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
