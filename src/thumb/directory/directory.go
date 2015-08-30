package directory

import (
	thumb ".."
	"../../fs"
	imageth "../image"
	"fmt"
	"image"
	"image/draw"
	"log"
	"math/rand"
)

func init() {
	thumb.RegisterThumber(DirectoryThumber{})
}

type DirectoryThumber struct{}

func (DirectoryThumber) Accepts(file *fs.File) bool {
	return file.Info.IsDir()
}

func (DirectoryThumber) IconThumb(file *fs.File, w, h int) (image.Image, error) {
	children, err := file.Children()
	if err != nil {
		return nil, err
	}

	iconNames := []string{
		".icon.png",
		".icon.jpg",
		".icon.jpeg",
	}
	for _, iconName := range iconNames {
		if icon, ok := children[iconName]; ok {
			return imageth.ImageThumber{}.Thumb(&icon, w, h)
		}
	}

	return nil, fmt.Errorf("Directory does not contain an icon file")
}

func (th DirectoryThumber) MosaicThumb(file *fs.File, w, h int) (image.Image, error) {
	children, err := file.Children()
	if err != nil {
		return nil, err
	}

	thumbableFiles := []fs.File{}
	for _, file := range children {
		// Yes, this is probably slow as shit, but it's fine for now.
		if len(thumbableFiles) > 50 {
			break // That's quite enough.
		}
		if thumb.FindThumber(&file) != nil {
			thumbableFiles = append(thumbableFiles, file)
		}
	}
	if len(thumbableFiles) == 0 {
		return nil, fmt.Errorf("No files to create a directory thumbnail")
	}

	var nCellsX int
	var nCellsY int
	if len(thumbableFiles) < 2*2 {
		nCellsX = 1
		nCellsY = 1
	} else if len(thumbableFiles) < 3*3 {
		nCellsX = 2
		nCellsY = 2
	} else {
		nCellsX = 3
		nCellsY = 3
	}
	cellW := w / nCellsX
	cellH := h / nCellsY

	dst := image.NewRGBA(image.Rectangle{
		Min: image.Point{0, 0},
		Max: image.Point{w, h},
	})

	retry := false
	for x := 0; x < nCellsX; x++ {
		for y := 0; y < nCellsY || retry; y++ {
			retry = false

			n := rand.Intn(len(thumbableFiles))
			cellFile := &thumbableFiles[n]
			thumbableFiles = append(thumbableFiles[:n], thumbableFiles[n+1:]...)
			if len(thumbableFiles) == 0 {
				return nil, fmt.Errorf("No files to create a directory thumbnail")
			}

			cell, err := thumb.FindThumber(cellFile).Thumb(cellFile, cellW, cellH)
			if err != nil {
				log.Println(err)
				retry = true // Retry
				continue
			}

			draw.Draw(dst, image.Rectangle{
				Min: image.Point{cellW * x, cellH * y},
				Max: image.Point{cellW*x + cellW, cellH*y + cellH},
			}, cell, image.Point{}, draw.Over)
		}
	}

	return dst, nil
}

func (th DirectoryThumber) Thumb(file *fs.File, w, h int) (image.Image, error) {
	if icon, err := th.IconThumb(file, w, h); err == nil {
		return icon, err
	}
	return th.MosaicThumb(file, w, h)
}
