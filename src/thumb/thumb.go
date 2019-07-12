package thumb

import (
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"time"

	"webfs/src/fs"
)

var thumbers []Thumber

func RegisterThumber(thumber Thumber) {
	thumbers = append(thumbers, thumber)
}

type Thumber interface {
	// Accepts checks wether the thumber is capable of creating a thumbnail of
	// the specified file.
	Accepts(filename string) (bool, error)
	// Thumb creates a thumbnail from a file with the specified dimensions.
	Thumb(filename string, w, h int) (image.Image, error)
}

func FindThumber(filename string) (Thumber, error) {
	var aerr error
	for _, th := range thumbers {
		ok, err := th.Accepts(filename)
		if err != nil {
			aerr = err
			continue
		}
		if ok {
			return th, nil
		}
	}
	return nil, aerr
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
			return fmt.Errorf("no thumber to generate thumbnail for %q", filename)
		}
		img, err := th.Thumb(filename, width, height)
		if err != nil {
			return err
		}
		return jpeg.Encode(wr, img, nil)
	})
}

func MimeType(filename string) (string, error) {
	fileMime := mime.TypeByExtension(path.Ext(filename))
	if fileMime != "" && fileMime != "application/octet-stream" {
		return fileMime, nil
	}

	fd, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer fd.Close()
	var buf [512]byte
	n, err := fd.Read(buf[:])
	if err != nil {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

// AcceptMimes is a convencience function to make writing the Accepts() method
// a bit easier. It takes a file and a set of mimetypes the file should match.
//
// This function attempts to determine the type using the filename and falls
// back to http.DetectContentType() if that does not work.
func AcceptMimes(filename string, mimes ...string) (bool, error) {
	fileMime, err := MimeType(filename)
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

func cacheInstance(w, h int) string {
	return fmt.Sprintf("%vx%v", w, h)
}
