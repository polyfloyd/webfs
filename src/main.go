package main

import (
	assets "./assets-go"
	"./fs"
	"./fs/filecache"
	"./fs/memcache"
	"./thumb"
	_ "./thumb/directory"
	_ "./thumb/image"
	_ "./thumb/video"
	"bytes"
	"encoding/json"
	"flag"
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
	"sync"
	"time"
)

const (
	PUBLIC   = "public"
	CONFFILE = "config.json"
)

const (
	THUMB_WIDTH  = 140
	THUMB_HEIGHT = 140
)

var (
	BUILD   = strings.Trim(string(assets.MustAsset("_BUILD")), "\n ")
	VERSION = strings.Trim(string(assets.MustAsset("_VERSION")), "\n ")
)

var (
	startTime     = time.Now()
	pageTemlates  = map[string]*template.Template{}
	authenticator Authenticator
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

func LoadConfig(filename string) (*Config, error) {
	config := &Config{}
	if in, err := os.Open(filename); err != nil {
		return nil, err
	} else if err := json.NewDecoder(in).Decode(&config); err != nil {
		return nil, err
	}
	return config, nil
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

	configFile := flag.String("conf", CONFFILE, "Path to the configuration file")
	preGenThumbs := flag.Bool("pregenthumbs", false, "Generate thumbnails for every file in all configured filesystems")
	var noPasswd *bool
	if BUILD == "debug" {
		noPasswd = flag.Bool("nopasswd", false, "Globally disable passord protection (debug builds only)")
	} else {
		noPasswd = new(bool)
	}
	flag.Parse()

	config, err := LoadConfig(*configFile)
	if err != nil {
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

	if *noPasswd {
		authenticator = NilAuthenticator{}
		log.Println("Password authentication disabled")
	} else {
		authenticator = BasicAuthenticator{}
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

		r.Path("/view/" + fsConf.Name + "/{path:.*}").HandlerFunc(htFsView(webfs, config))
		r.Path("/thumb/" + fsConf.Name + "/{path:.*}.jpg").HandlerFunc(htFsThumb(webfs))
		r.Path("/download/" + fsConf.Name + "/{path:.*}.zip").HandlerFunc(htFsDownload(webfs))
	}

	if *preGenThumbs {
		go pregenerateThumbnails(filesystems)
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

func pregenerateThumbnails(filesystems map[string]*fs.Filesystem) {
	numRunners := runtime.NumCPU() / 2
	if numRunners <= 0 {
		numRunners = 1
	}
	log.Printf("Generating thumbnails using %v workers", numRunners)

	fileStream := make(chan *fs.File)
	finished := make(chan bool)
	defer close(fileStream)
	defer close(finished)

	go func() {
		for _, fs := range filesystems {
			files, err := fs.Tree("/")
			if err != nil {
				log.Printf("Unable to generate thumbs for FS %v: %v", fs.Name, err)
			}
			log.Printf("Generating %v thumbs for %v", len(files), fs.Name)
			for _, file := range files {
				fileStream <- file
			}
		}
		for i := 0; i < numRunners; i++ {
			finished <- true
		}
	}()

	var wg sync.WaitGroup
	wg.Add(numRunners)
	for i := 0; i < numRunners; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case file := <-fileStream:
					if thumb, _, err := thumb.ThumbFile(file, THUMB_WIDTH, THUMB_HEIGHT); err != nil {
						log.Println(err)
					} else if thumb != nil {
						thumb.Close()
					}
				case <-finished:
					break
				}
			}
		}()
	}
	wg.Wait()
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

		if !authenticator.Authenticate(file, w, req) {
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
					"path": func() string {
						if child.Path[0] == '/' {
							return child.Path[1:]
						}
						return child.Path
					}(),
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

		cachedThumb, modTime, err := thumb.ThumbFile(file, THUMB_WIDTH, THUMB_HEIGHT)
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

func htFsDownload(webfs *fs.Filesystem) func(res http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		reqPath := path.Join("/", mux.Vars(req)["path"])
		file, err := webfs.Find(reqPath)
		if err != nil {
			panic(err)
		}

		if file == nil || file.IsDotfile() {
			http.NotFound(res, req)
			return
		}

		res.Header().Set("Content-Type", "application/zip")

		filter := func(file *fs.File) (bool, error) {
			if file.IsDotfile() {
				return false, nil
			}

			// TODO: Slow as shit, will fix later.
			// TODO: Currently, if subdirectories require authentication, they
			// are completely culled from the archive. It would be nice to
			// somehow include them.
			authenticated, _ := authenticator.IsAuthenticated(file, req)
			// Errors arising from IsAuthenticated() are ignored.
			return authenticated, nil
		}
		if err := fs.ZipTreeFilter(file, filter, res); err != nil {
			panic(err)
		}
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
