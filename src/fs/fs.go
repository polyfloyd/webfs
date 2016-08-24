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
	Fs   *Filesystem
}

func (file File) Open() (*os.File, error) {
	return os.Open(file.RealPath())
}

func (file File) MimeType() string {
	return mime.TypeByExtension(path.Ext(file.Path))
}

func (file File) RealPath() string {
	return path.Join(file.Fs.RealPath, file.Path)
}

func (file File) IsDotfile() bool {
	return file.Info.Name()[0] == '.'
}

func (file File) Parent() *File {
	parentPath := path.Dir(file.Path)
	if parentPath == file.Path {
		return nil
	}
	if parentPath == "." || parentPath == "/" {
		return &file.Fs.Root
	}

	info, err := os.Stat(parentPath)
	if err != nil {
		return nil
	}
	return &File{
		Info: info,
		Path: parentPath,
		Fs:   file.Fs,
	}
}

// Gets the directory contents of the file, or nil if the file is not a
// directory.
func (file File) Children() (map[string]*File, error) {
	if !file.Info.IsDir() {
		return nil, nil
	}

	fd, err := os.Open(file.RealPath())
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	index := map[string]*File{}
	children, err := fd.Readdir(-1)
	if err != nil {
		return index, nil
	}

	for _, child := range children {
		index[child.Name()] = &File{
			Info: child,
			Path: path.Join(file.Path, child.Name()),
			Fs:   file.Fs,
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
		RealPath: FixHome(path),
		Name:     name,
	}

	stat, err := os.Stat(fs.RealPath)
	if err != nil {
		return nil, err
	}

	fs.Root = File{
		Info: stat,
		Path: "/",
		Fs:   fs,
	}

	return fs, nil
}

func (fs *Filesystem) Find(p string) (*File, error) {
	if p == "" || p == "/" {
		return &fs.Root, nil
	}

	if s := p[:len(p)-1]; s == "/" {
		p = s
	}

	currentNode := &fs.Root
	for _, f := range strings.Split(p, "/")[1:] {
		children, err := currentNode.Children()
		if err != nil {
			return nil, err
		}

		if newNode, ok := children[f]; ok {
			currentNode = newNode
		} else {
			return nil, nil
		}
	}

	return currentNode, nil
}

func (fs *Filesystem) Tree(p string) ([]*File, error) {
	root, err := fs.Find(p)
	if err != nil {
		return nil, err
	}

	files := make([]*File, 0)

	var iterChildren func(*File) error
	iterChildren = func(file *File) error {
		children, err := file.Children()
		if err != nil {
			return err
		}
		for _, child := range children {
			files = append(files, child)
			if child.Info.IsDir() {
				if err := iterChildren(child); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := iterChildren(root); err != nil {
		return nil, err
	}
	return files, nil
}
