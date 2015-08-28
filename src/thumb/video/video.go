package video

import (
	thumb ".."
	"../../fs"
	"fmt"
	"image"
	"image/jpeg"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var acceptMimes = []string{
	"video/3gpp",
	"video/annodex",
	"video/dl",
	"video/dv",
	"video/fli",
	"video/gl",
	"video/mpeg",
	"video/MP2T",
	"video/mp4",
	"video/quicktime",
	"video/mp4v-es",
	"video/ogg",
	"video/parityfec",
	"video/pointer",
	"video/webm",
	"video/vnd.fvt",
	"video/vnd.motorola.video",
	"video/vnd.motorola.videop",
	"video/vnd.mpegurl",
	"video/vnd.mts",
	"video/vnd.nokia.interleaved-multimedia",
	"video/vnd.vivo",
	"video/x-flv",
	"video/x-la-asf",
	"video/x-mng",
	"video/x-ms-asf",
	"video/x-ms-wm",
	"video/x-ms-wmv",
	"video/x-ms-wmx",
	"video/x-ms-wvx",
	"video/x-msvideo",
	"video/x-sgi-movie",
	"video/x-matroska",
}

func init() {
	thumb.RegisterThumber(VideoThumber{})
}

func ffmpegDuration(dur time.Duration) string {
	return fmt.Sprintf("%v:%v:%v.%v",
		int64(dur/time.Hour),
		int64((dur%time.Hour)/time.Minute),
		int64((dur%time.Minute)/time.Second),
		int64(dur%time.Second/time.Microsecond),
	)
}

type VideoThumber struct{}

func (VideoThumber) Accepts(file *fs.File) bool {
	return thumb.AcceptMimes(file, acceptMimes...)
}

func (VideoThumber) videoDuration(file *fs.File) (time.Duration, error) {
	cmd := exec.Command("ffprobe",
		"-select_streams", "v:0",
		"-show_entries", "stream=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		file.RealPath(),
	)

	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(strings.Trim(string(out), "\n"), 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(float64(time.Second) * f), nil
}

func (vt VideoThumber) Thumb(file *fs.File, w, h int) (image.Image, error) {
	duration, err := vt.videoDuration(file)
	if err != nil {
		duration = time.Second // Take a guess and hope the video is longer than this.
	}

	cmd := exec.Command("ffmpeg",
		"-ss", ffmpegDuration(duration/2),
		"-i", file.RealPath(),
		"-vframes", "1",
		"-f", "image2",
		"-pix_fmt", "yuv420p",
		"-vf", fmt.Sprintf("scale=%v:-1", w),
		"-",
	)

	o, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	image, err := jpeg.Decode(o)
	if err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return image, nil
}
