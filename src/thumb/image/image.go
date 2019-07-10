package image

import (
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/nfnt/resize"

	"webfs/src/fs"
	"webfs/src/thumb"
)

func init() {
	thumb.RegisterThumber(ImageThumber{})
}

type ImageThumber struct{}

func (ImageThumber) Accepts(file *fs.File) bool {
	return thumb.AcceptMimes(file, "image/jpeg", "image/png", "image/gif")
}

func (ImageThumber) Thumb(file *fs.File, w, h int) (image.Image, error) {
	fd, err := os.Open(file.RealPath())
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	img, _, err := image.Decode(fd)
	if err != nil {
		return nil, err
	}

	var src image.Image
	if img.Bounds().Dx() > img.Bounds().Dy() {
		src = resize.Resize(0, uint(h), img, resize.NearestNeighbor)
	} else {
		src = resize.Resize(uint(w), 0, img, resize.NearestNeighbor)
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), src, image.Point{
		X: (src.Bounds().Dx() - w) / 2,
		Y: (src.Bounds().Dy() - h) / 2,
	}, draw.Over)
	return dst, nil

}
