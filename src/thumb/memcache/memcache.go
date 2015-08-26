package memcache

import (
	thumb ".."
	"../../fs"
	"bytes"
	"io"
	"sync"
	"time"
)

// An implementation of a ThumbCache storing all its thumbs in sytem memory.
// It's better not to use this when we have need to serve a metric crapload of
// files.
type ThumbMemCache struct {
	store map[cacheKey]*cachedThumb
	lock  sync.RWMutex
}

func NewCache() *ThumbMemCache {
	return &ThumbMemCache{
		store: map[cacheKey]*cachedThumb{},
	}
}

func (cache *ThumbMemCache) Get(file *fs.File, w, h int) (thumb.ReadSeekCloser, time.Time, error) {
	cache.lock.RLock()
	thumb, ok := cache.store[cacheKey{
		w:    w,
		h:    h,
		path: file.RealPath(),
	}]
	cache.lock.RUnlock()
	if !ok {
		return nil, time.Unix(0, 0), nil
	}

	// Wait if the thumbnail is being written. The lock will be realeased by
	// the Close() function of thumbReader.
	thumb.lock.RLock()
	return thumbReader{
		Reader: bytes.NewReader(thumb.buf.Bytes()),
		thumb:  thumb,
	}, thumb.modTime, nil
}

func (cache *ThumbMemCache) Put(file *fs.File, w, h int) (io.WriteCloser, error) {
	thumb := &cachedThumb{modTime: file.Info.ModTime()}
	// Lock now so we don't cause any race conditions with Get(). The lock is
	// released by the call to Close() of the returned writer.
	thumb.lock.Lock()

	cache.lock.Lock()
	cache.store[cacheKey{
		w:    w,
		h:    h,
		path: file.RealPath(),
	}] = thumb
	cache.lock.Unlock()

	return &thumbWriter{
		Buffer: &thumb.buf,
		thumb:  thumb,
	}, nil
}

func (cache *ThumbMemCache) Destroy(file *fs.File, w, h int) error {
	cache.lock.Lock()
	key := cacheKey{
		w:    w,
		h:    h,
		path: file.RealPath(),
	}

	if cth, ok := cache.store[key]; ok {
		cth.lock.Lock()
		delete(cache.store, key)
		cth.lock.Unlock()
	}

	cache.lock.Unlock()
	return nil
}

type cachedThumb struct {
	buf     bytes.Buffer
	modTime time.Time
	lock    sync.RWMutex
}

type cacheKey struct {
	w, h int
	path string
}

type thumbReader struct {
	*bytes.Reader
	thumb *cachedThumb
}

func (reader thumbReader) Close() error {
	reader.thumb.lock.RUnlock()
	return nil
}

type thumbWriter struct {
	*bytes.Buffer
	thumb *cachedThumb
}

func (writer thumbWriter) Close() error {
	writer.thumb.lock.Unlock()
	return nil
}
