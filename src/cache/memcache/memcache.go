package memcache

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"webfs/src/cache"
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

func (cache *MemCache) Get(filename string, instance string) (cache.ReadSeekCloser, time.Time, error) {
	cache.lock.RLock()
	cachedFile, ok := cache.store[cacheKey(filename, instance)]
	cache.lock.RUnlock()
	if !ok {
		return nil, time.Time{}, nil
	}

	// Wait if the file is being written. The lock will be released by the
	// Close() function of cachedFileReader.
	cachedFile.lock.RLock()
	return cachedFileReader{
		Reader: bytes.NewReader(cachedFile.buf.Bytes()),
		file:   cachedFile,
	}, cachedFile.modTime, nil
}

func (cache *MemCache) Put(filename string, instance string) (io.WriteCloser, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}

	cachedFile := &cachedFile{modTime: info.ModTime()}
	// Lock now so we don't cause any race conditions with Get(). The lock is
	// released by the call to Close() of the returned writer.
	cachedFile.lock.Lock()

	cache.lock.Lock()
	cache.store[cacheKey(filename, instance)] = cachedFile
	cache.lock.Unlock()

	return &cachedFileWriter{
		Buffer: &cachedFile.buf,
		file:   cachedFile,
	}, nil
}

func (cache *MemCache) Destroy(filename string, instance string) error {
	cache.lock.Lock()
	key := cacheKey(filename, instance)

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

func cacheKey(filename string, instance string) string {
	return fmt.Sprintf("%v-%v", filename, instance)
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
