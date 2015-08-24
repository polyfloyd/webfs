package image

import (
	thumb ".."
	"github.com/nfnt/resize"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
)

func init() {
	thumb.RegisterThumber(ImageThumber{})
}

type ImageThumber struct{}

func (this ImageThumber) Accepted() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
	}
}

func (this ImageThumber) Thumb(in io.Reader, out io.Writer, w, h int) error {
	img, _, err := image.Decode(in)
	if err != nil {
		return err
	}

	// Preserve aspect ratio by setting height to 0
	result := resize.Resize(uint(w), 0, img, resize.NearestNeighbor)
	jpeg.Encode(out, result, nil)

	return nil
}
