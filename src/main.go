package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"html/template"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
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
	directoryth "./thumb/directory"
	_ "./thumb/image"
	_ "./thumb/vector"
	_ "./thumb/video"
	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
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

type AssetServeHandler string

func (name AssetServeHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(string(name))))
	modTime := startTime
	if BUILD == "debug" {
		modTime = time.Now()
	}
	http.ServeContent(res, req, string(name), modTime, bytes.NewReader(assets.MustAsset(string(name))))
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
	var sessionBaseDir string
	if config.Cache != nil && *config.Cache != "" {
		cache, err := filecache.NewCache(path.Join(*config.Cache, "thumbs"), 0)
		if err != nil {
			log.Fatal(err)
		}
		thumbCache = cache
		sessionBaseDir = *config.Cache
	} else {
		thumbCache = memcache.NewCache()
		sessionBaseDir = os.TempDir()
	}

	if *noPasswd {
		authenticator = NilAuthenticator{}
		log.Println("Password authentication disabled")
	} else {
		auth, err := NewBasicAuthenticator(path.Join(sessionBaseDir, "sessions"))
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
		r.Path(urlPath).Handler(AssetServeHandler(file))
	}

	filesystems := map[string]*fs.Filesystem{}
	for _, fsConf := range config.FS {
		if _, ok := filesystems[fsConf.Name]; ok {
			log.Fatalf("Duplicate filesystem %q", fsConf.Name)
		}
		webfs, err := fs.NewFilesystem(strings.TrimSuffix(fsConf.Path, "/"), fsConf.Name)
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
		MaxHeaderBytes: 1 << 20,
		ReadTimeout:    10 * time.Second,
		// The timeout is set to this absurd value to make sure downloads don't
		// get aborted after the usual 10 seconds. The issue can not be fixed
		// right now due to limitations of the Go HTTP server.
		WriteTimeout: 2 * time.Hour,
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
	go func() {
		defer close(fileStream)
		for _, webfs := range filesystems {
			log.Printf("Generating thumbs for %q", webfs.Name)
			filepath.Walk(webfs.RealPath, func(path string, info os.FileInfo, err error) error {
				if path == webfs.RealPath || fs.IsDotFile(path) {
					return nil
				}
				fileStream <- &fs.File{
					Info: info,
					Path: strings.TrimPrefix(path, webfs.RealPath+"/"),
					Fs:   webfs,
				}
				return nil
			})
		}
	}()

	var wg sync.WaitGroup
	wg.Add(numRunners)
	for i := 0; i < numRunners; i++ {
		go func() {
			defer wg.Done()
			for file := range fileStream {
				log.Println(file.Path)
				if thumb, _, err := thumb.ThumbFile(thumbCache, file, THUMB_WIDTH, THUMB_HEIGHT); err != nil {
					log.Println(err)
				} else if thumb != nil {
					thumb.Close()
				}
			}
		}()
	}
	wg.Wait()
	log.Printf("Done generating thumbs")
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
					http.Error(res, "Unauthorized", http.StatusUnauthorized)
				}
				return
			}

			if !file.Info.IsDir() {
				if thumb.AcceptMimes(file, "image/jpeg", "image/png") {
					// Scale down the image to reduce transfer time to the client.
					const WIDTH, HEIGHT = 1366, 768
					cachedImage, modTime, err := fs.CacheFile(thumbCache, file, "view", func(file *fs.File, wr io.Writer) error {
						fd, err := os.Open(file.RealPath())
						if err != nil {
							return err
						}
						defer fd.Close()
						img, _, err := image.Decode(fd)
						if err != nil {
							return err
						}
						resized := resize.Thumbnail(WIDTH, HEIGHT, img, resize.NearestNeighbor)
						return jpeg.Encode(wr, resized, nil)
					})

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
				if fs.IsDotFile(child.Path) {
					continue // Hide dotfiles
				}
				isUnlocked, err := authenticator.IsUnlocked(child, req)
				if err != nil {
					log.Println(err)
					isUnlocked = false
				}

				files = append(files, map[string]interface{}{
					"name": name,
					"path": child.Path,
					"type": func() string {
						if child.Info.IsDir() {
							return "directory"
						} else {
							return fs.MimeType(child.RealPath())
						}
					}(),
					"hasThumb": (isUnlocked || directoryth.HasIconThumb(child)) && thumb.FindThumber(child) != nil,
					"hasPassword": func() bool {
						hasPassword, err := authenticator.HasPassword(child)
						if err != nil {
							log.Println(err)
							return true
						}
						return hasPassword
					}(),
					"isUnlocked": isUnlocked,
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
		if file == nil || fs.IsDotFile(file.Path) {
			http.NotFound(res, req)
			return
		}

		renderFile(file)
	}
}

func htFsThumb(webfs *fs.Filesystem, thumbCache fs.Cache) func(w http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		p := path.Join("/", mux.Vars(req)["path"])
		file, err := webfs.Find(p)
		if err != nil {
			panic(err)
		}

		if file == nil || fs.IsDotFile(file.Path) {
			http.NotFound(res, req)
			return
		}

		if auth, err := authenticator.IsUnlocked(file, req); err != nil {
			panic(err)
		} else if !auth && !directoryth.HasIconThumb(file) {
			http.Error(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		cachedThumb, modTime, err := thumb.ThumbFile(thumbCache, file, THUMB_WIDTH, THUMB_HEIGHT)
		if err != nil {
			log.Println(err)
			http.NotFound(res, req)
			return
		}
		if cachedThumb == nil {
			http.NotFound(res, req)
			return
		}
		defer cachedThumb.Close()

		res.Header().Set("Content-Type", "image/jpeg")
		http.ServeContent(res, req, file.Info.Name(), modTime, cachedThumb)
	}
}

func htFsGet(webfs *fs.Filesystem) func(res http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		file, err := webfs.Find(path.Join("/", mux.Vars(req)["path"]))
		if err != nil {
			panic(err)
		}
		if file == nil || fs.IsDotFile(file.Path) {
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

func htFsDownload(webfs *fs.Filesystem) func(res http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		file, err := webfs.Find(path.Join("/", mux.Vars(req)["path"]))
		if err != nil {
			panic(err)
		}

		if file == nil || fs.IsDotFile(file.Path) {
			http.NotFound(res, req)
			return
		}

		res.Header().Set("Content-Type", "application/zip")
		if file.Path == "/" {
			res.Header().Set("Content-Disposition", "attachment; filename=\""+webfs.Name+".zip\"")
		}

		filter := func(file *fs.File) (bool, error) {
			if fs.IsDotFile(file.Path) {
				return false, nil
			}
			return authenticator.IsUnlocked(file, req)
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
		if tmpl, ok := pageTemplates[name]; ok {
			return tmpl
		} else {
			pageTemplates[name] = template.Must(template.New(name).Parse(string(assets.MustAsset(name))))
			return pageTemplates[name]
		}
	}
}
