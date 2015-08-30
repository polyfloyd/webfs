package main

import (
	assets "./assets-go"
	"./fs"
	"./thumb"
	_ "./thumb/directory"
	"./thumb/filecache"
	_ "./thumb/image"
	"./thumb/memcache"
	_ "./thumb/video"
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"html/template"
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
	startTime    = time.Now()
	pageTemlates = map[string]*template.Template{}
)

type Config struct {
	Address string `json:"address"`
	URLRoot string `json:"urlroot"`

	Cache *string `json:"cache"`

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
	modTime := startTime
	if BUILD == "debug" {
		modTime = time.Now()
	}
	http.ServeContent(w, req, h.name, modTime, bytes.NewReader(assets.MustAsset(h.name)))
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

	if config.Cache != nil {
		cache, err := filecache.NewCache(path.Join(*config.Cache, "thumbs"), 0)
		if err != nil {
			log.Fatal(err)
		}
		thumb.SetCache(cache)
	} else {
		thumb.SetCache(memcache.NewCache())
	}

	r := mux.NewRouter()

	for _, file := range assets.AssetNames() {
		if !strings.HasPrefix(file, PUBLIC) {
			continue
		}
		urlPath := strings.TrimPrefix(file, PUBLIC)
		r.Path(urlPath).Handler(&AssetServeHandler{name: file})
	}

	filesystems := map[string]*fs.Filesystem{}
	for _, fsConf := range config.FS {
		if _, ok := filesystems[fsConf.Name]; ok {
			log.Fatalf("Duplicate filesystem \"%v\"", fsConf.Name)
		}
		webfs, err := fs.NewFilesystem(fsConf.Path, fsConf.Name)
		if err != nil {
			log.Fatal(err)
		}
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

func htFsView(fs *fs.Filesystem, config *Config) func(w http.ResponseWriter, req *http.Request) {
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
		reqPath := path.Join("/", mux.Vars(req)["path"])
		file, err := fs.Find(reqPath)
		if err != nil {
			panic(err)
		}

		if file == nil || file.IsDotfile() {
			http.NotFound(w, req)
			return
		}

		if !Authenticate(file, w, req) {
			return
		}

		children, err := file.Children()
		if err != nil {
			panic(err)
		}

		if children == nil {
			http.ServeFile(w, req, file.RealPath())

		} else {
			files := []map[string]interface{}{}
			for name, child := range children {
				if child.IsDotfile() {
					continue // Hide dotfiles
				}
				files = append(files, map[string]interface{}{
					"name": name,
					"path": child.Path,
					"type": func() string {
						if child.Info.IsDir() {
							return "directory"
						} else if spl := strings.Split(child.MimeType(), "/"); len(spl) == 2 {
							// Use the first part of the mime as filetype.
							return spl[0]
						} else {
							return "generic"
						}
					}(),
					"hasThumb": true, // TODO
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
				"path":  reqPath,
				"files": files,
			})
			if err != nil {
				panic(err)
			}
		}
	}
}

func htFsThumb(fs *fs.Filesystem) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		p := path.Join("/", mux.Vars(req)["path"])
		file, err := fs.Find(p)
		if err != nil {
			panic(err)
		}

		if file == nil || file.IsDotfile() {
			http.NotFound(w, req)
			return
		}

		// TODO
		// We don't check for password files when serving thumbnails for now.
		// Not a whole lot of info can be leaked in 140x140 images. Especially
		// with the program's current usage.

		const width = 140
		const height = 140

		cachedThumb, modTime, err := thumb.ThumbFile(file, width, height)
		if err != nil {
			log.Println(err)
			http.NotFound(w, req)
			return
		}
		if cachedThumb == nil {
			http.NotFound(w, req)
			return
		}
		defer cachedThumb.Close()

		http.ServeContent(w, req, file.Info.Name(), modTime, cachedThumb)
	}
}

func htFsGet(webfs *fs.Filesystem) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("get")) // TODO
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
