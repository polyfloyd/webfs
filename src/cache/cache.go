package cache

import (
	"bytes"
	"io"
	"os"
	"time"
)

type ReadSeekCloser interface {
	io.Closer
	io.Reader
	io.Seeker
}

// Specifies a caching mechanism for keeping instances of files in a cache.
// The implementation should be thread-safe.
type Cache interface {
	// Gets the cached instance of the file, or nil and an error if it does not exists.
	// An error will also be set if a read error occurs in the implementation.
	//
	// If a file matching the instance criteria is in the process of being
	// stored by Put(), this function will block until the file is available or
	// an error occurs, in which case nil is returned and an error is set.
	//
	// The returned time is the creation time of the file, always later
	// than the modifacation time of the file on disk.
	Get(filename string, instance string) (ReadSeekCloser, time.Time, error)

	// Stores a file by providing the writer the instance should be written
	// to.
	Put(filename string, instance string) (io.WriteCloser, error)

	// Removes a cached file. If the instance identifier is "" all instances
	// are removed. This function is a no-op if no file exists.
	Destroy(filename string, instance string) error
}

func CacheFile(cache Cache, filename, instance string, getContents func(string, io.Writer) error) (ReadSeekCloser, time.Time, error) {
	cached, modTime, err := cache.Get(filename, instance)
	if err != nil {
		return nil, time.Time{}, err
	}

	info, err := os.Stat(filename)
	if err != nil {
		return nil, time.Time{}, err
	}

	if cached == nil || info.ModTime().After(modTime) {
		cacheWriter, err := cache.Put(filename, instance)
		if err != nil {
			cache.Destroy(filename, instance)
			return nil, time.Time{}, err
		}

		buf := &bufSeekCloser{}
		if err := getContents(filename, io.MultiWriter(&buf.buf, cacheWriter)); err != nil {
			cacheWriter.Close()
			cache.Destroy(filename, instance)
			return nil, time.Time{}, err
		}
		cacheWriter.Close()

		buf.Reader = bytes.NewReader(buf.buf.Bytes())
		return buf, info.ModTime(), nil
	}
	return cached, modTime, nil
}

type bufSeekCloser struct {
	buf bytes.Buffer
	*bytes.Reader
}

func (bufSeekCloser) Close() error {
	return nil
}
