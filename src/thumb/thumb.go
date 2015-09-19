package thumb

import (
	"../fs"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime"
	"net/http"
	"path"
	"time"
)

var (
	thumbers   []Thumber
	thumbCache fs.Cache
)

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

// This is the preferred way of creating a thumbnail. This function will manage
// the cache set by SetCache() and update the thumbnail if the file
// modification time changes.
//
// The thumbnail is exposed as a JPEG image.
func ThumbFile(file *fs.File, width, height int) (fs.ReadSeekCloser, time.Time, error) {
	cachedThumb, modTime, err := thumbCache.Get(file, cacheInstance(width, height))
	if err != nil {
		return nil, time.Unix(0, 0), err
	}

	if cachedThumb == nil || file.Info.ModTime().After(modTime) {
		th := FindThumber(file)
		if th == nil {
			return nil, time.Unix(0, 0), nil
		}

		cacheWriter, err := thumbCache.Put(file, cacheInstance(width, height))
		if err != nil {
			return nil, time.Unix(0, 0), err
		}

		img, err := th.Thumb(file, width, height)
		if err != nil {
			cacheWriter.Close()
			thumbCache.Destroy(file, cacheInstance(width, height))
			return nil, time.Unix(0, 0), err
		}

		buf := &bufSeekCloser{}
		jpeg.Encode(io.MultiWriter(&buf.buf, cacheWriter), img, nil)
		cacheWriter.Close()
		buf.Reader = bytes.NewReader(buf.buf.Bytes())
		return buf, file.Info.ModTime(), nil
	}

	return cachedThumb, modTime, nil
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

func SetCache(cache fs.Cache) {
	thumbCache = cache
}
