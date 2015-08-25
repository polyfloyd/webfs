package fs

import (
	"mime"
	"os"
	"path"
	"strings"
)

type File struct {
	Info os.FileInfo
	Path string
	fs   *Filesystem
}

func (file File) Open() (*os.File, error) {
	return os.Open(file.RealPath())
}

func (file File) MimeType() string {
	return mime.TypeByExtension(path.Ext(file.Path))
}

func (file File) RealPath() string {
	return path.Join(file.fs.RealPath, file.Path)
}

// Gets the directory contents of the file, or nil if the file is not a
// directory.
func (file File) Children() (map[string]File, error) {
	if !file.Info.IsDir() {
		return nil, nil
	}

	fd, err := os.Open(file.RealPath())
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	index := map[string]File{}
	children, err := fd.Readdir(-1)
	if err != nil {
		return index, nil
	}

	for _, child := range children {
		index[child.Name()] = File{
			Info: child,
			Path: path.Join(file.Path, child.Name()),
			fs:   file.fs,
		}
	}
	return index, nil
}

type Filesystem struct {
	RealPath string
	Name     string
	Root     File
}

func NewFilesystem(path, name string) (*Filesystem, error) {
	fs := &Filesystem{
		RealPath: path,
		Name:     name,
	}

	stat, err := os.Stat(fs.RealPath)
	if err != nil {
		return nil, err
	}

	fs.Root = File{
		Info: stat,
		Path: "/",
		fs:   fs,
	}

	return fs, nil
}

func (fs *Filesystem) Find(p string) (*File, error) {
	if s := p[:len(p)-1]; s == "/" {
		p = s
	}

	if p == "/" {
		return &fs.Root, nil
	}

	currentNode := &fs.Root
	for _, f := range strings.Split(p, "/")[1:] {
		children, err := currentNode.Children()
		if err != nil {
			return nil, err
		}

		if newNode, ok := children[f]; ok {
			currentNode = &newNode
		} else {
			return nil, nil
		}
	}

	return currentNode, nil
}
