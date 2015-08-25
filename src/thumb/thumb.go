package thumb

import (
	"../fs"
	"image"
	"io"
	"mime"
	"net/http"
	"path"
)

var thumbers []Thumber

type Thumber interface {
	// Checks wether the thumber is capable of creating a thumbnail of the
	// specified file.
	Accepts(file *fs.File) bool

	// Creates a thumbnail with the specified dimensions.
	Thumb(file *fs.File, w, h int) (image.Image, error)
}

func RegisterThumber(thumber Thumber) {
	thumbers = append(thumbers, thumber)
}

// Convencience function to make writing the Accepts() method a bit easier.
// Takes a file and a set of mimetypes the file should match.
//
// This function attempts to determine the type using the filename and falls
// back to http.DetectContentType() if that does not work.
func AcceptMimes(file *fs.File, mimes ...string) bool {
	fileMime := mime.TypeByExtension(path.Ext(file.Path))
	if fileMime == "" || fileMime == "application/octet-stream" {
		fd, err := file.Open()
		if err != nil {
			return false
		}
		defer fd.Close()
		var buf [512]byte
		n, _ := fd.Read(buf[:])
		fileMime = http.DetectContentType(buf[:n])
	}

	for _, mimetype := range mimes {
		if fileMime == mimetype {
			return true
		}
	}
	return false
}

func FindThumber(file *fs.File) Thumber {
	for _, th := range thumbers {
		if th.Accepts(file) {
			return th
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
