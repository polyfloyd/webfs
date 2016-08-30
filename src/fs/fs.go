package fs

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
)

type File struct {
	Info os.FileInfo
	Path string
	Fs   *Filesystem
}

func (file File) RealPath() string {
	return path.Join(file.Fs.RealPath, file.Path)
}

func (file File) Parent() *File {
	parentPath := path.Dir(file.Path)
	if parentPath == file.Path {
		return nil
	}
	if parentPath == "" || parentPath == "." || parentPath == "/" {
		return &file.Fs.Root
	}

	parent := &File{
		Path: parentPath,
		Fs:   file.Fs,
	}
	info, err := os.Stat(parent.RealPath())
	if err != nil {
		return nil
	}
	parent.Info = info
	return parent
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
		RealPath: ResolveHome(path),
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

func IsDotFile(filename string) bool {
	return path.Base(filename)[0] == '.'
}

func MimeType(filename string) string {
	fileMime := mime.TypeByExtension(path.Ext(filename))
	if fileMime != "" && fileMime != "application/octet-stream" {
		return fileMime
	}

	fd, err := os.Open(filename)
	if err != nil {
		return "application/octet-stream"
	}
	defer fd.Close()
	var buf [512]byte
	n, _ := fd.Read(buf[:])
	return http.DetectContentType(buf[:n])
}

func ResolveHome(p string) string {
	if len(p) == 0 || p[0] != '~' {
		return p
	}
	home := os.Getenv("HOME")
	if home == "" {
		log.Fatal("~ found in path, but $HOME is not set")
	}
	return path.Join(home, p[1:])
}
