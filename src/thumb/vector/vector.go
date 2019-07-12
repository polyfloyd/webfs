package vector

import (
	"image"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"

	"webfs/src/thumb"
)

func init() {
	if _, err := exec.LookPath("inkscape"); err != nil {
		log.Printf("Disabling vector thumber: %v", err)
		return
	}
	thumb.RegisterThumber(VectorThumber{})
}

type VectorThumber struct{}

func (VectorThumber) Accepts(filename string) (bool, error) {
	return thumb.AcceptMimes(filename,
		"application/pdf",
		"application/postscript",
		"image/svg+xml",
	)
}

func (VectorThumber) Thumb(filename string, w, h int) (image.Image, error) {
	tmp, err := ioutil.TempFile("", "webfs_vecthumb_")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	cmd := exec.Command(
		"inkscape",
		"--file", filename,
		"--export-png", tmp.Name(),
		"--export-background", "white",
		"--export-width", strconv.Itoa(w),
		"--export-height", strconv.Itoa(h),
		"--export-area-drawing",
	)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	pngFile, err := os.Open(tmp.Name())
	if err != nil {
		return nil, err
	}
	defer pngFile.Close()
	return png.Decode(pngFile)
}
