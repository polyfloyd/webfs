package thumb

import (
	"../fs"
	"bytes"
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
	thumbCache Cache
)

type ReadSeekCloser interface {
	io.Closer
	io.Reader
	io.Seeker
}

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
func ThumbFile(file *fs.File, width, height int) (ReadSeekCloser, time.Time, error) {
	cachedThumb, modTime, err := thumbCache.Get(file, width, height)
	if err != nil {
		return nil, time.Unix(0, 0), err
	}

	if cachedThumb == nil || file.Info.ModTime().After(modTime) {
		th := FindThumber(file)
		if th == nil {
			return nil, time.Unix(0, 0), nil
		}

		cacheWriter, err := thumbCache.Put(file, width, height)
		if err != nil {
			return nil, time.Unix(0, 0), err
		}

		img, err := th.Thumb(file, width, height)
		if err != nil {
			cacheWriter.Close()
			thumbCache.Destroy(file, width, height)
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

type bufSeekCloser struct {
	buf bytes.Buffer
	*bytes.Reader
}

func (bufSeekCloser) Close() error {
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
	//
	// The returned time is the creation time of the thumbnail, always later
	// than the modifacation time of the file.
	Get(file *fs.File, w, h int) (ReadSeekCloser, time.Time, error)

	// Stores a thumbnail by providing the writer the thumbnail image should be
	// written to.
	Put(file *fs.File, w, h int) (io.WriteCloser, error)

	// Removes a cached thumbnail. If w and h are 0, all thumbnails of this
	// file are removed. This function is a no-op if no such thumbnail exists.
	Destroy(file *fs.File, w, h int) error
}

func SetCache(cache Cache) {
	thumbCache = cache
}
