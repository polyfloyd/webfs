package image

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	thumb ".."
	"../../fs"
	"github.com/nfnt/resize"
)

func init() {
	thumb.RegisterThumber(ImageThumber{})
}

type ImageThumber struct{}

func (ImageThumber) Accepts(file *fs.File) bool {
	return thumb.AcceptMimes(file, "image/jpeg", "image/png", "image/gif")
}

func (ImageThumber) Thumb(file *fs.File, w, h int) (image.Image, error) {
	fd, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	img, _, err := image.Decode(fd)
	if err != nil {
		return nil, err
	}

	// Preserve aspect ratio by setting height to 0
	return resize.Thumbnail(uint(w), uint(h), img, resize.NearestNeighbor), nil
}
