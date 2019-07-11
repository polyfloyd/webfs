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
	Accepts(filename string) (bool, error)

	// Creates a thumbnail with the specified dimensions.
	Thumb(filename string, w, h int) (image.Image, error)
}

func RegisterThumber(thumber Thumber) {
	thumbers = append(thumbers, thumber)
}

// Convencience function to make writing the Accepts() method a bit easier.
// Takes a file and a set of mimetypes the file should match.
//
// This function attempts to determine the type using the filename and falls
// back to http.DetectContentType() if that does not work.
func AcceptMimes(filename string, mimes ...string) (bool, error) {
	fileMime, err := fs.MimeType(filename)
	if err != nil {
		return false, err
	}
	for _, mimetype := range mimes {
		if fileMime == mimetype {
			return true, nil
		}
	}
	return false, nil
}

func FindThumber(filename string) (Thumber, error) {
	for _, th := range thumbers {
		if ok, err := th.Accepts(filename); err != nil {
			return nil, err
		} else if ok {
			return th, nil
		}
	}
	return nil, nil
}

// This is the preferred way of creating a thumbnail. This function will manage
// the cache set by SetCache() and update the thumbnail if the file
// modification time changes.
//
// The thumbnail is exposed as a JPEG image.
func ThumbFile(thumbCache fs.Cache, filename string, width, height int) (fs.ReadSeekCloser, time.Time, error) {
	return fs.CacheFile(thumbCache, filename, cacheInstance(width, height), func(filename string, wr io.Writer) error {
		th, err := FindThumber(filename)
		if err != nil {
			return err
		} else if th == nil {
			return fmt.Errorf("No thumber to generate thumbnail for %q", filename)
		}
		img, err := th.Thumb(filename, width, height)
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
