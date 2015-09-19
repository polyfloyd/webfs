package memcache

import (
	fs ".."
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"
)

// An implementation of a file Cache storing all its files in sytem memory.
// It's better not to use this when we have need to serve a metric crapload of
// files.
type MemCache struct {
	store map[string]*cachedFile
	lock  sync.RWMutex
}

func NewCache() *MemCache {
	return &MemCache{
		store: map[string]*cachedFile{},
	}
}

func (cache *MemCache) Get(file *fs.File, instance string) (fs.ReadSeekCloser, time.Time, error) {
	cache.lock.RLock()
	cachedFile, ok := cache.store[cacheKey(file, instance)]
	cache.lock.RUnlock()
	if !ok {
		return nil, time.Unix(0, 0), nil
	}

	// Wait if the file is being written. The lock will be released by the
	// Close() function of cachedFileReader.
	cachedFile.lock.RLock()
	return cachedFileReader{
		Reader: bytes.NewReader(cachedFile.buf.Bytes()),
		file:   cachedFile,
	}, cachedFile.modTime, nil
}

func (cache *MemCache) Put(file *fs.File, instance string) (io.WriteCloser, error) {
	cachedFile := &cachedFile{modTime: file.Info.ModTime()}
	// Lock now so we don't cause any race conditions with Get(). The lock is
	// released by the call to Close() of the returned writer.
	cachedFile.lock.Lock()

	cache.lock.Lock()
	cache.store[cacheKey(file, instance)] = cachedFile
	cache.lock.Unlock()

	return &cachedFileWriter{
		Buffer: &cachedFile.buf,
		file:   cachedFile,
	}, nil
}

func (cache *MemCache) Destroy(file *fs.File, instance string) error {
	cache.lock.Lock()
	key := cacheKey(file, instance)

	if cth, ok := cache.store[key]; ok {
		cth.lock.Lock()
		delete(cache.store, key)
		cth.lock.Unlock()
	}

	cache.lock.Unlock()
	return nil
}

type cachedFile struct {
	buf     bytes.Buffer
	modTime time.Time
	lock    sync.RWMutex
}

func cacheKey(file *fs.File, instance string) string {
	return fmt.Sprintf("%v-%v", file.RealPath(), instance)
}

type cachedFileReader struct {
	*bytes.Reader
	file *cachedFile
}

func (reader cachedFileReader) Close() error {
	reader.file.lock.RUnlock()
	return nil
}

type cachedFileWriter struct {
	*bytes.Buffer
	file *cachedFile
}

func (writer cachedFileWriter) Close() error {
	writer.file.lock.Unlock()
	return nil
}
