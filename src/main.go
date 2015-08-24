package main

import (
	assets "./assets-go"
	"./thumb"
	_ "./thumb/image"
	memcache "./thumb/memcache"
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	PUBLIC   = "public"
	CONFFILE = "config.json"
)

var (
	BUILD   = strings.Trim(string(assets.MustAsset("BUILD")), "\n ")
	VERSION = strings.Trim(string(assets.MustAsset("VERSION")), "\n ")
)

var (
	pageTemlates = map[string]*template.Template{}
)

type Config struct {
	Address string `json:"address"`
	URLRoot string `json:"urlroot"`

	Piwik       bool   `json:"piwik"`
	PiwikRoot   string `json:"piwikroot"`
	PiwikSiteID int    `json:"piwiksiteid"`

	FS []struct {
		Path string `json:"path"`
		Name string `json:"name"`
	} `json:"fs"`
}

type AssetServeHandler struct {
	name string
}

func (h *AssetServeHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(h.name)))
	io.Copy(w, bytes.NewReader(assets.MustAsset(h.name)))
}

func main() {
	log.Printf("Version: %v (%v)\n", VERSION, BUILD)
	runtime.GOMAXPROCS(runtime.NumCPU())

	config := Config{}
	if in, err := os.Open(CONFFILE); err != nil {
		log.Fatal(err)
	} else if err := json.NewDecoder(in).Decode(&config); err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()

	for _, file := range assets.AssetNames() {
		if !strings.HasPrefix(file, PUBLIC) {
			continue
		}
		urlPath := strings.TrimPrefix(file, PUBLIC)
		r.Path(urlPath).Handler(&AssetServeHandler{name: file})
	}

	filesystems := map[string]*Filesystem{}
	for _, fsConf := range config.FS {
		if _, ok := filesystems[fsConf.Name]; ok {
			log.Fatalf("Duplicate filesystem \"%v\"", fsConf.Name)
		}
		webfs := NewFilesystem(fsConf.Path, fsConf.Name)
		filesystems[fsConf.Name] = webfs

		r.Path("/fs/" + fsConf.Name + "/view/{path:.*}").HandlerFunc(htFsView(webfs, &config))
		r.Path("/fs/" + fsConf.Name + "/thumb/{path:.*}").HandlerFunc(htFsThumb(webfs))
		r.Path("/fs/" + fsConf.Name + "/get/{path:.*}").HandlerFunc(htFsGet(webfs))
	}

	log.Printf("Now accepting HTTP connections on %v", config.Address)
	server := &http.Server{
		Addr:           config.Address,
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(server.ListenAndServe())
}

func htFsView(fs *Filesystem, config *Config) func(w http.ResponseWriter, req *http.Request) {
	static := map[string][]string{
		"js":  []string{},
		"css": []string{},
	}
	for _, file := range assets.AssetNames() {
		if !strings.HasPrefix(file, PUBLIC) {
			continue
		}
		urlPath := strings.TrimPrefix(file, PUBLIC)

		switch path.Ext(file) {
		case ".css":
			static["css"] = append(static["css"], urlPath)
		case ".js":
			static["js"] = append(static["js"], urlPath)
		}
	}
	for _, a := range static {
		sort.Strings(a)
	}

	return func(w http.ResponseWriter, req *http.Request) {
		p := path.Join("/", mux.Vars(req)["path"])
		file := fs.Find(p)

		if file == nil {
			http.NotFound(w, req)
			return
		}

		if fi, ok := file.(*File); ok {
			if fd, err := fi.Open(); err != nil {
				panic(err)
			} else {
				defer fd.Close()
				io.Copy(w, fd)
			}

		} else if dir, ok := file.(map[string]interface{}); ok {
			names := []map[string]interface{}{}
			for name := range dir {
				names = append(names, map[string]interface{}{
					"name":     name,
					"path":     path.Join(p, name),
					"type":     "generic", // TODO
					"hasThumb": true,      // TODO
				})
			}

			err := getPageTemplate("main.html").Execute(w, map[string]interface{}{
				"build":   BUILD,
				"version": VERSION,

				"urlroot": config.URLRoot,
				"assets":  static,
				"time":    time.Now(),

				"piwik":       config.Piwik,
				"piwikRoot":   config.PiwikRoot,
				"piwikSiteID": config.PiwikSiteID,

				"fs":    fs,
				"path":  p,
				"files": names,
			})
			if err != nil {
				panic(err)
			}
		}
	}
}

func htFsThumb(fs *Filesystem) func(w http.ResponseWriter, req *http.Request) {
	var cache thumb.Cache = memcache.NewCache()

	return func(w http.ResponseWriter, req *http.Request) {
		p := path.Join("/", mux.Vars(req)["path"])
		file := fs.Find(p)

		if file == nil {
			http.NotFound(w, req)
			return
		}

		if fi, ok := file.(*File); ok {
			const width = 140
			const height = 140
			cachedThumb, err := cache.Get(fi.path, width, height)
			if err != nil {
				panic(err)
			}

			if cachedThumb == nil {
				// Attempt to generate a thumbnail for the requested file.
				if th := thumb.FindThumber(fi.path); th != nil {
					origFileReader, err := fi.Open()
					if err != nil {
						panic(err)
					}
					defer origFileReader.Close()

					cacheWriter := cache.Put(fi.path, width, height)
					defer cacheWriter.Close()
					thumbTee := io.MultiWriter(w, cacheWriter)
					if err := th.Thumb(origFileReader, thumbTee, width, height); err != nil {
						panic(err)
					}
				}

			} else {
				defer cachedThumb.Close()
				if _, err := io.Copy(w, cachedThumb); err != nil {
					panic(err)
				}
			}
		}
	}
}

func htFsGet(webfs *Filesystem) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("get"))
	}
}

func getPageTemplate(name string) *template.Template {
	if BUILD == "debug" {
		return template.Must(template.New(name).Parse(string(assets.MustAsset(name))))
	} else {
		if tmpl, ok := pageTemlates[name]; ok {
			return tmpl
		} else {
			pageTemlates[name] = template.Must(template.New(name).Parse(string(assets.MustAsset(name))))
			return pageTemlates[name]
		}
	}
}

func htMainPage(config *Config, static map[string][]string) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		err := getPageTemplate("main.html").Execute(w, map[string]interface{}{
			"build":   BUILD,
			"version": VERSION,

			"urlroot": config.URLRoot,
			"assets":  static,
			"time":    time.Now(),

			"piwik":       config.Piwik,
			"piwikRoot":   config.PiwikRoot,
			"piwikSiteID": config.PiwikSiteID,
		})
		if err != nil {
			panic(err)
		}
	}
}
