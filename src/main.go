package main

import (
	"bytes"
	"encoding/json"
	"flag"
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

	assets "./assets-go"
	"./fs"
	"./fs/filecache"
	"./fs/memcache"
	"./thumb"
	_ "./thumb/directory"
	_ "./thumb/image"
	_ "./thumb/vector"
	_ "./thumb/video"
	"github.com/gorilla/mux"
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

// Global vars are bad, but these are not supposed to be changed.
var (
	startTime     = time.Now()
	pageTemplates = map[string]*template.Template{}
	authenticator Authenticator
	config        *Config
	staticAssets  map[string][]string
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

	configFile := flag.String("conf", CONFFILE, "Path to the configuration file")
	preGenThumbs := flag.Bool("pregenthumbs", false, "Generate thumbnails for every file in all configured filesystems")
	var noPasswd *bool
	if BUILD == "debug" {
		noPasswd = flag.Bool("nopasswd", false, "Globally disable passord protection (debug builds only)")
	} else {
		noPasswd = new(bool)
	}
	flag.Parse()

	var err error
	config, err = LoadConfig(*configFile)
	if err != nil {
		log.Fatal(err)
	}

	staticAssets = genStaticAssets()

	var thumbCache fs.Cache
	if config.Cache != nil && *config.Cache != "" {
		cache, err := filecache.NewCache(path.Join(*config.Cache, "thumbs"), 0)
		if err != nil {
			log.Fatal(err)
		}
		thumbCache = cache
	} else {
		thumbCache = memcache.NewCache()
	}

	if *noPasswd {
		authenticator = NilAuthenticator{}
		log.Println("Password authentication disabled")
	} else {
		auth, err := NewBasicAuthenticator(path.Join(*config.Cache, "sessions"))
		if err != nil {
			log.Fatal(err)
		}
		authenticator = auth
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
			log.Fatalf("Duplicate filesystem %q", fsConf.Name)
		}
		webfs, err := fs.NewFilesystem(fsConf.Path, fsConf.Name)
		if err != nil {
			log.Fatal(err)
		}
		filesystems[fsConf.Name] = webfs

		r.Path("/view/" + fsConf.Name + "/{path:.*}").HandlerFunc(htFsView(webfs, thumbCache))
		r.Path("/thumb/" + fsConf.Name + "/{path:.*}.jpg").HandlerFunc(htFsThumb(webfs, thumbCache))
		r.Path("/get/" + fsConf.Name + "/{path:.*}").HandlerFunc(htFsGet(webfs))
		r.Path("/download/" + fsConf.Name + "/{path:.*}.zip").HandlerFunc(htFsDownload(webfs))
	}

	if *preGenThumbs {
		go pregenerateThumbnails(filesystems, thumbCache)
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

func pregenerateThumbnails(filesystems map[string]*fs.Filesystem, thumbCache fs.Cache) {
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
					if thumb, _, err := thumb.ThumbFile(thumbCache, file, THUMB_WIDTH, THUMB_HEIGHT); err != nil {
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

func baseTeplateArgs() map[string]interface{} {
	return map[string]interface{}{
		"build":   BUILD,
		"version": VERSION,

		"urlroot": config.URLRoot,
		"assets":  staticAssets,
		"time":    time.Now(),

		"piwik":       config.Piwik,
		"piwikRoot":   config.PiwikRoot,
		"piwikSiteID": config.PiwikSiteID,
	}
}

func genStaticAssets() map[string][]string {
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
	return static
}

func htFsView(webfs *fs.Filesystem, thumbCache fs.Cache) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		var renderFile func(*fs.File)
		renderFile = func(file *fs.File) {
			auth, err := authenticator.Authenticate(file, res, req)
			if err != nil {
				panic(err)
			}
			if !auth {
				if parent := file.Parent(); parent != nil {
					renderFile(parent)
				} else {
					res.Write([]byte("Unauthorized"))
				}
				return
			}

			if !file.Info.IsDir() {
				if thumb.AcceptMimes(file, "image/jpeg", "image/png") {
					// Scale down the image to reduce transfer time to the client.
					const WIDTH, HEIGHT = 1366, 768
					cachedImage, modTime, err := thumb.ThumbFile(thumbCache, file, WIDTH, HEIGHT)
					if err != nil {
						log.Println(err)
						http.NotFound(res, req)
						return
					}
					if cachedImage == nil {
						http.NotFound(res, req)
						return
					}
					defer cachedImage.Close()
					http.ServeContent(res, req, file.Info.Name(), modTime, cachedImage)

				} else {
					http.ServeFile(res, req, file.RealPath())
				}
				return
			}

			children, err := file.Children()
			if err != nil {
				panic(err)
			}

			files := make([]map[string]interface{}, len(children))[0:0]
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
					"hasThumb": thumb.FindThumber(child) != nil,
				})
			}

			args := baseTeplateArgs()
			args["files"] = files
			args["fs"] = webfs
			args["path"] = file.Path
			args["title"] = path.Base(file.Path)
			if err := getPageTemplate("main.html").Execute(res, args); err != nil {
				panic(err)
			}
		}

		file, err := webfs.Find(path.Join("/", mux.Vars(req)["path"]))
		if err != nil {
			panic(err)
		}
		if file == nil || file.IsDotfile() {
			http.NotFound(res, req)
			return
		}

		renderFile(file)
	}
}

func htFsThumb(fs *fs.Filesystem, thumbCache fs.Cache) func(w http.ResponseWriter, req *http.Request) {
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

		cachedThumb, modTime, err := thumb.ThumbFile(thumbCache, file, THUMB_WIDTH, THUMB_HEIGHT)
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

		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeContent(w, req, file.Info.Name(), modTime, cachedThumb)
	}
}

func htFsGet(webfs *fs.Filesystem) func(res http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		file, err := webfs.Find(path.Join("/", mux.Vars(req)["path"]))
		if err != nil {
			panic(err)
		}

		if file == nil || file.IsDotfile() {
			http.NotFound(res, req)
			return
		}

		if auth, err := authenticator.Authenticate(file, res, req); err != nil {
			panic(err)
		} else if !auth {
			res.Write([]byte("Unauthorized"))
			return
		}

		http.ServeFile(res, req, file.RealPath())
	}
}

type abortZipper struct{}

func (abortZipper) Error() string {
	return "abort zipper"
}

func htFsDownload(webfs *fs.Filesystem) func(res http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		file, err := webfs.Find(path.Join("/", mux.Vars(req)["path"]))
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
			authenticated, _ := authenticator.Authenticate(file, res, req)
			if !authenticated {
				return false, abortZipper{}
			}
			return authenticated, nil
		}
		if err := fs.ZipTreeFilter(file, filter, res); err != nil {
			if _, ok := err.(abortZipper); ok {
				return
			}
			panic(err)
		}
	}
}

func getPageTemplate(name string) *template.Template {
	if BUILD == "debug" {
		return template.Must(template.New(name).Parse(string(assets.MustAsset(name))))
	} else {
		if tmpl, ok := pageTemplates[name]; ok {
			return tmpl
		} else {
			pageTemplates[name] = template.Must(template.New(name).Parse(string(assets.MustAsset(name))))
			return pageTemplates[name]
		}
	}
}
