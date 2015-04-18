package main

import (
	"io"
	"mime"
	"os"
	"path"
	"strings"
)

type File struct {
	Info  os.FileInfo
	fs   *Filesystem
	path  string
}

func (this *File) Open() (io.ReadCloser, error) {
	return os.Open(path.Join(this.fs.RealPath, this.path))
}

func (this *File) MimeType() string {
	return mime.TypeByExtension(path.Ext(this.path))
}


type Filesystem struct {
	RealPath string
	Name     string
	Files    map[string]interface{}
}

func NewFilesystem(path, name string) *Filesystem {
	fs := Filesystem{
		RealPath: path,
		Name:     name,
		Files:    map[string]interface{}{},
	}
	fs.reloadFiles()
	return &fs
}

/**
 * Recursively reloads the file index of this filesystem PHP style, meaning
 * that any errors encountered are completely ignored.
 */
func (fs *Filesystem) reloadFiles() {
	var dir func(realpath string) map[string]interface{}
	dir = func(realpath string) (index map[string]interface{}) {
		index = map[string]interface{}{}

		file, err := os.Open(path.Join(fs.RealPath, realpath))
		if err != nil {
			return
		}
		defer file.Close()

		files, _ := file.Readdir(-1)
		for _, fi := range files {
			p := path.Join(realpath, fi.Name())
			if fi.IsDir() {
				index[fi.Name()] = dir(p)
			} else {
				index[fi.Name()] = &File{
					Info: fi,
					fs:   fs,
					path: p,
				}
			}
		}

		return
	}

	fs.Files = dir("/")
}

func (fs *Filesystem) Find(p string) interface{} {
	if s := p[:len(p)-1]; s == "/" {
		p = s
	}

	if p == "/"{
		return fs.Files
	}

	current := fs.Files
	for _, f := range strings.Split(p, "/")[1:] {
		if found, ok := current[f]; ok {
			if dir, ok := found.(map[string]interface{}); ok {
				current = dir
				continue
			} else {
				return found
			}
		}
		return nil
	}
	return current
}

func (fs *Filesystem) FindDir(path string) map[string]interface{} {
	if maybeDir := fs.Find(path); maybeDir != nil {
		if dir, ok := maybeDir.(map[string]interface{}); ok {
			return dir
		}
	}
	return map[string]interface{}{}
}

func (fs *Filesystem) FindFile(p string) *File {
	if maybeFile := fs.Find(p); maybeFile != nil {
		if file, ok := maybeFile.(*File); ok {
			return file
		}
	}
	return nil
}
