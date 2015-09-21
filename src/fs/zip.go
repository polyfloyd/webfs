package fs

import (
	"archive/zip"
	"io"
	"path"
	"strings"
)

func ZipTree(file *File, wr io.Writer) error {
	all := func(file *File) (bool, error) {
		return true, nil
	}
	return ZipTreeFilter(file, all, wr)
}

func ZipTreeFilter(file *File, filter func(file *File) (bool, error), wr io.Writer) error {
	var files []File

	var addFiles func(file *File) error
	addFiles = func(file *File) error {
		if include, err := filter(file); err != nil {
			return err
		} else if !include {
			return nil
		}
		if !file.Info.IsDir() {
			files = append(files, *file)
		}

		children, err := file.Children()
		if err != nil {
			return err
		}
		if children == nil {
			return nil
		}

		for _, child := range children {
			if err := addFiles(&child); err != nil {
				return nil
			}
		}
		return nil
	}

	addFiles(file)
	return ZipFiles(files, path.Dir(file.Path), wr)
}

func ZipFiles(files []File, stripPrefix string, wr io.Writer) error {
	zipper := zip.NewWriter(wr)

	for _, file := range files {
		header, err := zip.FileInfoHeader(file.Info)
		if err != nil {
			return err
		}

		header.Name = strings.TrimPrefix(strings.TrimPrefix(file.Path, stripPrefix), "/")
		entry, err := zipper.CreateHeader(header)
		if err != nil {
			return err
		}

		fd, err := file.Open()
		if err != nil {
			return err
		}
		defer fd.Close()

		if _, err = io.Copy(entry, fd); err != nil {
			return err
		}
	}

	return zipper.Close()
}
