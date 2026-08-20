package storage

import (
	"image"
	"image/jpeg"
	"io"

	"golang.org/x/image/draw"
)

const maxPosterWidth = 2000

// encodeJPEG writes img as a JPEG with the given quality.
func encodeJPEG(w io.Writer, img image.Image, quality int) error {
	return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
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
