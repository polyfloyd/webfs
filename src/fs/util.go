package fs

import (
	"log"
	"os"
	"path"
)

func FixHome(p string) string {
	if p[0] == '~' {
		home := os.Getenv("HOME")
		if home == "" {
			log.Println("~ found in path, but $HOME is not set")
			return p
		}
		return path.Join(home, p[1:])
	}
	return p
}
