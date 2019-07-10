package thumb

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"time"

	"webfs/src/fs"
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
	fileMime := fs.MimeType(file.RealPath())
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

// This is the preferred way of creating a thumbnail. This function will manage
// the cache set by SetCache() and update the thumbnail if the file
// modification time changes.
//
// The thumbnail is exposed as a JPEG image.
func ThumbFile(thumbCache fs.Cache, file *fs.File, width, height int) (fs.ReadSeekCloser, time.Time, error) {
	return fs.CacheFile(thumbCache, file, cacheInstance(width, height), func(file *fs.File, wr io.Writer) error {
		th := FindThumber(file)
		if th == nil {
			return fmt.Errorf("No thumber to generate thumbnail for %q", file.Path)
		}
		img, err := th.Thumb(file, width, height)
		if err != nil {
			return err
		}
		return jpeg.Encode(wr, img, nil)
	})
}

func cacheInstance(w, h int) string {
	return fmt.Sprintf("%vx%v", w, h)
}

type bufSeekCloser struct {
	buf bytes.Buffer
	*bytes.Reader
}

func (bufSeekCloser) Close() error {
	return nil
}
