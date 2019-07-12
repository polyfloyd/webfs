package directory

import (
	"fmt"
	"image"
	"image/draw"
	"math/rand"
	"os"
	"path/filepath"

	"webfs/src/thumb"
	imageth "webfs/src/thumb/image"
)

var iconNames = []string{
	".icon.png",
	".icon.jpg",
	".icon.jpeg",
}

func init() {
	thumb.RegisterThumber(DirectoryThumber{})
}

type DirectoryThumber struct{}

func (DirectoryThumber) Accepts(filename string) (bool, error) {
	stat, err := os.Stat(filename)
	if err != nil {
		return false, err
	}
	return stat.IsDir(), nil
}

func (DirectoryThumber) Thumb(filename string, w, h int) (image.Image, error) {
	if icon, err := IconThumb(filename, w, h); err == nil {
		return icon, nil
	}
	return MosaicThumb(filename, w, h)
}

func iconThumbFile(dirname string) (string, error) {
	fd, err := os.Open(dirname)
	if err != nil {
		return "", err
	}
	defer fd.Close()
	files, err := fd.Readdir(-1)
	if err != nil {
		return "", err
	}

	for _, f := range files {
		for _, iconName := range iconNames {
			if f.Name() == iconName {
				return filepath.Join(dirname, f.Name()), nil
			}
		}
	}
	return "", nil
}

func HasIconThumb(filename string) (bool, error) {
	iconFile, err := iconThumbFile(filename)
	if err != nil {
		return false, err
	}
	return iconFile != "", nil
}

func IconThumb(filename string, w, h int) (image.Image, error) {
	iconFile, err := iconThumbFile(filename)
	if err != nil {
		return nil, err
	}
	return imageth.ImageThumber{}.Thumb(iconFile, w, h)
}

func MosaicThumb(filename string, w, h int) (image.Image, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	files, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}

	thumbableFiles := make([]string, 50)[0:0]
	for _, f := range files {
		th, err := thumb.FindThumber(filepath.Join(filename, f.Name()))
		if err != nil {
			return nil, err
		}
		if th != nil {
			thumbableFiles = append(thumbableFiles, filepath.Join(filename, f.Name()))
		}
		if len(thumbableFiles) == cap(thumbableFiles) {
			break
		}
	}

	if len(thumbableFiles) == 0 {
		return nil, fmt.Errorf("no files to create a directory thumbnail")
	}

	var nCellsX int
	var nCellsY int
	if len(thumbableFiles) == 1 {
		nCellsX = 1
		nCellsY = 1
	} else if len(thumbableFiles) == 2 {
		nCellsX = 1
		nCellsY = 2
	} else if len(thumbableFiles) == 3 {
		nCellsX = 1
		nCellsY = 3
	} else if len(thumbableFiles) < 3*3 {
		nCellsX = 2
		nCellsY = 2
	} else {
		nCellsX = 3
		nCellsY = 3
	}
	cellW := w / nCellsX
	cellH := h / nCellsY

	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	for x := 0; x < nCellsX; x++ {
		for y := 0; y < nCellsY; y++ {
			if len(thumbableFiles) == 0 {
				return nil, fmt.Errorf("all files exhausted while trying to create a directory thumbnail")
			}
			n := rand.Intn(len(thumbableFiles))
			cellFile := thumbableFiles[n]
			thumbableFiles = append(thumbableFiles[:n], thumbableFiles[n+1:]...)

			th, err := thumb.FindThumber(cellFile)
			if err != nil {
				return nil, fmt.Errorf("error while drawing cell: %v", err)
			}
			cell, err := th.Thumb(cellFile, cellW, cellH)
			if err != nil {
				return nil, fmt.Errorf("error while drawing cell: %v", err)
			}

			draw.Draw(dst, image.Rectangle{
				Min: image.Point{X: cellW * x, Y: cellH * y},
				Max: image.Point{X: cellW*x + cellW, Y: cellH*y + cellH},
			}, cell, cell.Bounds().Min, draw.Over)
		}
	}

	return dst, nil
}
