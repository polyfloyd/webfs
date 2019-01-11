package vector

import (
	"image"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"

	"github.com/polyfloyd/webfs/src/fs"
	"github.com/polyfloyd/webfs/src/thumb"
)

func init() {
	th := VectorThumber{}
	if _, err := exec.LookPath("inkscape"); err == nil {
		thumb.RegisterThumber(th)
		return
	}

	log.Println("Disabling vector thumber, inkscape not found in PATH")
}

type VectorThumber struct{}

func (VectorThumber) Accepts(file *fs.File) bool {
	return thumb.AcceptMimes(file,
		"application/pdf",
		"application/postscript",
		"image/svg+xml",
	)
}

func (VectorThumber) Thumb(file *fs.File, w, h int) (image.Image, error) {
	tmp, err := ioutil.TempFile("", "webfs_vecthumb_")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	cmd := exec.Command(
		"inkscape",
		"--file", file.RealPath(),
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
