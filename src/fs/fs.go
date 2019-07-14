package fs

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"webfs/src/cache"
	"webfs/src/thumb"
	"webfs/src/thumb/directory"
)

var (
	ErrFileDoesNotExist   = fmt.Errorf("file does not exist")
	ErrNeedAuthentication = fmt.Errorf("authentication is needed to access this file")
	ErrNoThumbnail        = fmt.Errorf("file has no thumbnail")
)

type Authenticator interface {
	IsAuthenticated(filename string) error
}

type AuthenticatorFunc func(filename string) error

func (fn AuthenticatorFunc) IsAuthenticated(filename string) error {
	return fn(filename)
}

type File struct {
	Info    os.FileInfo
	Path    string
	RelPath string
}

func (f File) Name() string {
	return filepath.Base(f.Path)
}

type Filesystem struct {
	mount      string
	thumbCache cache.Cache
}

func NewFilesystem(mount string, thumbCache cache.Cache) (*Filesystem, error) {
	if !filepath.IsAbs(mount) {
		m, err := filepath.Abs(mount)
		if err != nil {
			return nil, err
		}
		mount = m
	}
	if stat, err := os.Stat(mount); err != nil {
		return nil, err
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("filesystem mount must be a directory")
	}
	return &Filesystem{mount: mount, thumbCache: thumbCache}, nil
}

// View returns:
// * directory: []File
// * file: File
func (fs *Filesystem) View(path string, auth Authenticator) (interface{}, error) {
	filename := fs.realPath(path)
	if isDotFile(filename) {
		return "", ErrFileDoesNotExist
	}
	if err := auth.IsAuthenticated(filename); err != nil {
		return nil, err
	}

	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return nil, ErrFileDoesNotExist
	} else if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return File{
			Info:    info,
			Path:    filename,
			RelPath: path,
		}, nil
	}

	fd, err := os.Open(filename)
	if os.IsNotExist(err) {
		return nil, ErrFileDoesNotExist
	} else if err != nil {
		return nil, err
	}
	defer fd.Close()

	osFiles, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}
	files := make([]File, 0, len(osFiles))
	for _, info := range osFiles {
		if !isDotFile(info.Name()) {
			files = append(files, File{
				Info:    info,
				Path:    filepath.Join(filename, info.Name()),
				RelPath: filepath.Join(path, info.Name()),
			})
		}
	}
	return files, nil
}

func (fs *Filesystem) FirstAccessibleParent(path string, auth Authenticator) (string, error) {
	filename := fs.realPath(path)
	if isDotFile(filename) {
		return "", ErrFileDoesNotExist
	}
	for strings.HasPrefix(filename, fs.mount) {
		if err := auth.IsAuthenticated(filename); err == nil {
			return strings.TrimPrefix(filename, fs.mount), nil
		} else if err != ErrNeedAuthentication {
			return "", err
		}
		filename = filepath.Dir(filename)
	}
	return "", ErrNeedAuthentication
}

func (fs *Filesystem) FileInfo(path string, auth Authenticator) (os.FileInfo, error) {
	filename := fs.realPath(path)
	if isDotFile(filename) {
		return nil, ErrFileDoesNotExist
	}
	if err := auth.IsAuthenticated(filename); err != nil {
		return nil, err
	}
	return os.Stat(filename)
}

func (fs *Filesystem) Filepath(path string, auth Authenticator) (string, error) {
	filename := fs.realPath(path)
	if isDotFile(filename) {
		return "", ErrFileDoesNotExist
	}
	if err := auth.IsAuthenticated(filename); err != nil {
		return "", err
	}
	return filename, nil
}

func (fs *Filesystem) Thumbnail(path string, w, h int, auth Authenticator) (cache.ReadSeekCloser, string, time.Time, error) {
	filename := fs.realPath(path)
	if isDotFile(filename) {
		return nil, "", time.Time{}, ErrFileDoesNotExist
	}
	if err := auth.IsAuthenticated(filename); err == ErrNeedAuthentication {
		if ok, err := directory.HasIconThumb(filename); err != nil {
			return nil, "", time.Time{}, err
		} else if !ok {
			return nil, "", time.Time{}, ErrNeedAuthentication
		}
	} else if err != nil {
		return nil, "", time.Time{}, err
	}

	cachedThumb, modTime, err := thumb.ThumbFile(fs.thumbCache, filename, w, h)
	if err != nil {
		return nil, "", time.Time{}, err
	} else if cachedThumb == nil {
		return nil, "", time.Time{}, ErrNoThumbnail
	}
	return cachedThumb, "image/jpeg", modTime, nil
}

func (fs *Filesystem) Zip(path string, wr io.Writer, auth Authenticator) error {
	filename := fs.realPath(path)
	if isDotFile(filename) {
		return ErrFileDoesNotExist
	}
	if err := auth.IsAuthenticated(filename); err != nil {
		return err
	}

	zipper := zip.NewWriter(wr)
	defer zipper.Close()

	return filepath.Walk(filename, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || isDotFile(path) {
			return nil
		}

		if err := auth.IsAuthenticated(path); err == ErrNeedAuthentication {
			return nil
		} else if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = strings.TrimPrefix(path, fs.mount)

		entry, err := zipper.CreateHeader(header)
		if err != nil {
			return err
		}

		fd, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fd.Close()
		_, err = io.Copy(entry, fd)
		return err
	})
}

func (fs *Filesystem) Mount() string {
	return fs.mount
}

func (fs *Filesystem) PregenerateThumbnails(w, h int) {
	numRunners := runtime.NumCPU() / 2
	if numRunners <= 0 {
		numRunners = 1
	}
	log.Printf("Generating thumbnails using %v workers", numRunners)

	fileStream := make(chan string)
	go func() {
		defer close(fileStream)
		log.Printf("Generating thumbs")
		filepath.Walk(fs.mount, func(path string, info os.FileInfo, err error) error {
			if path == fs.mount || isDotFile(path) {
				return nil
			}
			fileStream <- path
			return nil
		})
	}()

	var wg sync.WaitGroup
	wg.Add(numRunners)
	for i := 0; i < numRunners; i++ {
		go func() {
			defer wg.Done()
			for filename := range fileStream {
				log.Println(filename)
				if thumb, _, err := thumb.ThumbFile(fs.thumbCache, filename, w, h); err != nil {
					log.Println(err)
				} else if thumb != nil {
					thumb.Close()
				}
			}
		}()
	}
	wg.Wait()
	log.Printf("Done generating thumbs")
}

func (fs *Filesystem) RealPath(path string) string {
	return fs.realPath(path)
}

func (fs *Filesystem) realPath(path string) string {
	cleaned := filepath.Clean("/" + path)
	return filepath.Join(fs.mount, cleaned)
}

func isDotFile(path string) bool {
	b := filepath.Base(path)
	return len(b) > 0 && b[0] == '.'
}
