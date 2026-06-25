package storage

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime/multipart"

	"github.com/vegidio/avif-go"
	"golang.org/x/image/draw"
)

const maxPosterWidth = 2000

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

func processImage(file *multipart.FileHeader) (*bytes.Buffer, string, error) {
	src, err := file.Open()
	if err != nil {
		return nil, "", err
	}
	defer src.Close()

	img, _, err := image.Decode(src)
	if err != nil {
		return nil, "", err
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w > maxPosterWidth {
		ratio := float64(maxPosterWidth) / float64(w)
		newH := int(float64(h) * ratio)
		resized := image.NewRGBA(image.Rect(0, 0, maxPosterWidth, newH))
		catmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
		img = resized
	}

	var buf bytes.Buffer
	if err := avif.Encode(&buf, img, &avif.Options{
		Speed:        6,
		ColorQuality: 65,
		AlphaQuality: 60,
	}); err != nil {
		return nil, "", err
	}

	return &buf, ".avif", nil
}
