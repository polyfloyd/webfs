package fs

import (
	"archive/zip"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func ZipTreeFilter(file *File, filter func(file *File) (bool, error), wr io.Writer) error {
	zipper := zip.NewWriter(wr)
	defer zipper.Close()

	stripPrefix := path.Dir(file.RealPath())
	addPrefix := ""
	if file.Path == "/" {
		addPrefix = file.Fs.Name + "/"
		stripPrefix = file.RealPath()
	}

	return filepath.Walk(file.RealPath(), func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		include, err := filter(&File{
			Info: info,
			Path: strings.TrimPrefix(path, file.Fs.RealPath+"/"),
			Fs:   file.Fs,
		})
		if err != nil || !include {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = addPrefix + strings.TrimPrefix(strings.TrimPrefix(path, stripPrefix), "/")

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
