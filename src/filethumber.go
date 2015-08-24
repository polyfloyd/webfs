package main

import (
	"github.com/nfnt/resize"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"path"
)

var fileThumbers []FileThumber

func init() {
	RegisterFileThumber(&ImageThumber{})
}

type FileThumber interface {
	Accepted() []string
	Thumb(in io.Reader, out io.Writer, w, h int) error
}

func RegisterFileThumber(thumber FileThumber) {
	fileThumbers = append(fileThumbers, thumber)
}

func FindFileThumber(file *File) FileThumber {
	fileMime := mime.TypeByExtension(path.Ext(file.path))
	for _, th := range fileThumbers {
		for _, mime := range th.Accepted() {
			if mime == fileMime {
				return th
			}
		}
	}
	return nil
}

type ImageThumber struct{}

func (this *ImageThumber) Accepted() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
	}
}

func (this *ImageThumber) Thumb(in io.Reader, out io.Writer, w, h int) error {
	img, _, err := image.Decode(in)
	if err != nil {
		return err
	}

	// Preserve aspect ratio by setting height to 0
	result := resize.Resize(uint(w), 0, img, resize.NearestNeighbor)
	jpeg.Encode(out, result, nil)

	return nil
}
