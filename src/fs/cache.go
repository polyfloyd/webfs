package fs

import (
	"io"
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
	// than the modifacation time of the file.
	Get(file *File, instance string) (ReadSeekCloser, time.Time, error)

	// Stores a file by providing the writer the instance should be written
	// to.
	Put(file *File, instance string) (io.WriteCloser, error)

	// Removes a cached file. If the instance identifier is "" all instances
	// are removed. This function is a no-op if no file thumbnail exists.
	Destroy(file *File, instance string) error
}
