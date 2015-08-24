package thumb

import (
	"io"
	"mime"
	"path"
)

var thumbers []Thumber

type Thumber interface {
	// Returns a list of mimetypes this thumber accepts.
	Accepted() []string

	// Creates a thumbnail with the specified dimensions. It is assumed that
	// all thumbers produce their thumbnails in JPEG format.
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

// Specifies a caching mechanism for keeping thumbnails without having to
// regenerate them every single time one is requested.
// The implementation should be thread-safe.
type Cache interface {
	// Gets the cached instance of the file with the specified dimensions, or
	// nil and an error if it does not exists.
	// An error will also be set if a read error occurs in the implementation.
	//
	// If a thumbnail matching the criteria is in the process of being stored
	// by Put(), this function will block until the thumbnail is available or
	// an error occurs, in which case nil is returned and an error is set.
	Get(filepath string, w, h int) (io.ReadCloser, error)

	// Stores a thumbnail by providing the writer the thumbnail image should be
	// written to.
	Put(filepath string, w, h int) io.WriteCloser

	// Removes a cached thumbnail. If w and h are 0, all thumbnails of this
	// file are removed. This function is a no-op if no such thumbnail exists.
	Destroy(filepath string, w, h int) error
}
