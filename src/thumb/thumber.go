package thumb

import (
	"io"
	"mime"
	"path"
)

var thumbers []Thumber

type Thumber interface {
	Accepted() []string
	Thumb(in io.Reader, out io.Writer, w, h int) error
}

func RegisterThumber(thumber Thumber) {
	thumbers = append(thumbers, thumber)
}

func FindThumber(filepath string) Thumber {
	fileMime := mime.TypeByExtension(path.Ext(filepath))
	for _, th := range thumbers {
		for _, mime := range th.Accepted() {
			if mime == fileMime {
				return th
			}
		}
	}
	return nil
}
