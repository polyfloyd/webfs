package filecache

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"
	"webfs/src/cache"
)

// A cache using the filesystem as storage.
type ThumbFileCache struct {
	dir   string
	perm  os.FileMode
	lock  sync.RWMutex
	locks map[string]*sync.RWMutex
}

func NewCache(dir string, perm os.FileMode) (*ThumbFileCache, error) {
	if perm == 0 {
		perm = 0700 | os.ModeTemporary
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return nil, err
	}

	cache := &ThumbFileCache{
		dir:   dir,
		perm:  perm,
		locks: map[string]*sync.RWMutex{},
	}

	fd, err := os.Open(cache.dir)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	filenames, err := fd.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	for _, name := range filenames {
		cache.locks[path.Join(dir, name)] = &sync.RWMutex{}
	}

	return cache, nil
}

func (cache *ThumbFileCache) Get(filename string, instance string) (cache.ReadSeekCloser, time.Time, error) {
	cacheFile := cache.filename(filename, instance)

	cache.lock.RLock()
	defer cache.lock.RUnlock()
	lock, ok := cache.locks[cacheFile]
	if !ok {
		return nil, time.Time{}, nil
	}
	lock.RLock()

	fd, err := os.Open(cacheFile)
	if err != nil {
		lock.RUnlock()
		return nil, time.Time{}, err
	}

	info, err := os.Stat(cacheFile)
	if err != nil {
		lock.RUnlock()
		return nil, time.Time{}, err
	}

	return fileReleaser{
		File: fd,
		lock: lock.RLocker(),
	}, info.ModTime(), nil
}

func (cache *ThumbFileCache) Put(filename string, instance string) (io.WriteCloser, error) {
	cacheFile := cache.filename(filename, instance)

	cache.lock.Lock()
	defer cache.lock.Unlock()
	lock := &sync.RWMutex{}
	lock.Lock()
	cache.locks[cacheFile] = lock

	fd, err := os.OpenFile(cacheFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, cache.perm)
	if err != nil {
		return nil, err
	}

	return fileReleaser{
		File: fd,
		lock: lock,
	}, nil
}

func (cache *ThumbFileCache) Destroy(filename string, instance string) error {
	cacheFile := cache.filename(filename, instance)

	cache.lock.Lock()
	defer cache.lock.Unlock()
	lock, ok := cache.locks[cacheFile]
	if ok {
		lock.Lock()
		delete(cache.locks, cacheFile)
	}

	return os.Remove(cacheFile)
}

func (cache *ThumbFileCache) filename(filename string, instance string) string {
	return path.Join(cache.dir, fmt.Sprintf("%x-%v", sha1.Sum([]byte(filename)), instance))
}

type fileReleaser struct {
	*os.File
	lock sync.Locker
}

func (fr fileReleaser) Close() error {
	defer fr.lock.Unlock()
	return fr.File.Close()
}
