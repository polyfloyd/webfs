package main

import (
	"bytes"
	"flag"
	"fmt"
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

	"github.com/gorilla/mux"
	"github.com/nfnt/resize"

	"github.com/polyfloyd/webfs/src/assets"
	"github.com/polyfloyd/webfs/src/fs"
	"github.com/polyfloyd/webfs/src/fs/filecache"
	"github.com/polyfloyd/webfs/src/fs/memcache"
	"github.com/polyfloyd/webfs/src/thumb"
	directoryth "github.com/polyfloyd/webfs/src/thumb/directory"
	_ "github.com/polyfloyd/webfs/src/thumb/image"
	_ "github.com/polyfloyd/webfs/src/thumb/vector"
	_ "github.com/polyfloyd/webfs/src/thumb/video"
)

const (
	PUBLIC = "public"
)

const (
	THUMB_WIDTH  = 140
	THUMB_HEIGHT = 140
)

var (
	build       = "%BUILD%"
	version     = "%VERSION%"
	versionDate = "%VERSION_DATE%"
	buildDate   = "%BUILD_DATE%"
)

// Global vars are bad, but these are not supposed to be changed.
var (
	startTime     = time.Now()
	pageTemplates = map[string]*template.Template{}
	authenticator Authenticator
	staticAssets  map[string][]string
)

var (
	urlRoot     string
	piwikRoot   string
	piwikSiteID int
)

type AssetServeHandler string

func (name AssetServeHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(string(name))))
	modTime := startTime
	if build == "debug" {
		modTime = time.Now()
	}
	http.ServeContent(res, req, string(name), modTime, bytes.NewReader(assets.MustAsset(string(name))))
}

func main() {
	log.Printf("Version: %v (%v)\n", version, build)

	listenAddress := flag.String("listen", "localhost:8080", "The HTTP root of a Piwik installation, must not end with a slash")
	flag.StringVar(&urlRoot, "urlroot", "", "The HTTP root, must not end with a slash")
	flag.StringVar(&piwikRoot, "piwik-root", "", "The HTTP root of a Piwik installation, must not end with a slash")
	flag.IntVar(&piwikSiteID, "piwik-site", 0, "The Piwik Site ID")
	pregenThumbs := flag.Bool("pregen-thumbs", false, "Generate thumbnails for every file in all configured filesystems on startup")
	defaultCacheDir := path.Join(os.TempDir(), fmt.Sprintf("webfs-%d", os.Getuid()))
	cacheDir := flag.String("cache-dir", defaultCacheDir, "The directory to store generated thumbnails. If empty, all files are kept in memory")
	mountPath := flag.String("mount", ".", "The root directory to expose")
	var noPasswd *bool
	if build == "debug" {
		noPasswd = flag.Bool("nopasswd", false, "Globally disable passord protection (debug builds only)")
	} else {
		noPasswd = new(bool)
	}
	flag.Parse()

	if urlRoot == "" {
		urlRoot = fmt.Sprintf("http://%s", *listenAddress)
	}

	staticAssets = genStaticAssets()

	var thumbCache fs.Cache
	var sessionBaseDir string
	if *cacheDir != "" {
		cache, err := filecache.NewCache(path.Join(*cacheDir, "thumbs"), 0)
		if err != nil {
			log.Fatal(err)
		}
		thumbCache = cache
		sessionBaseDir = *cacheDir
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
	name := "mount"

	if _, ok := filesystems[name]; ok {
		log.Fatalf("Duplicate filesystem %q", name)
	}
	webfs, err := fs.NewFilesystem(strings.TrimSuffix(*mountPath, "/"), name)
	if err != nil {
		log.Fatal(err)
	}
	filesystems[name] = webfs

	r.Path("/view/" + name + "/{path:.*}").HandlerFunc(htFsView(webfs, thumbCache))
	r.Path("/thumb/" + name + "/{path:.*}.jpg").HandlerFunc(htFsThumb(webfs, thumbCache))
	r.Path("/get/" + name + "/{path:.*}").HandlerFunc(htFsGet(webfs))
	r.Path("/download/" + name + "/{path:.*}.zip").HandlerFunc(htFsDownload(webfs))

	if *pregenThumbs {
		go pregenerateThumbnails(filesystems, thumbCache)
	}

	log.Printf("Now accepting HTTP connections on %v", *listenAddress)
	server := &http.Server{
		Addr:           *listenAddress,
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
		"build":       build,
		"buildDate":   buildDate,
		"version":     version,
		"versionDate": versionDate,

		"urlroot": urlRoot,
		"assets":  staticAssets,
		"time":    time.Now(),

		"piwik":       piwikRoot != "" && piwikSiteID != 0,
		"piwikRoot":   piwikRoot,
		"piwikSiteID": piwikSiteID,
	}
}

func genStaticAssets() map[string][]string {
	static := map[string][]string{
		"js":  {},
		"css": {},
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

		file, err := webfs.Find(path.Clean("/" + mux.Vars(req)["path"]))
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
		file, err := webfs.Find(path.Clean("/" + mux.Vars(req)["path"]))
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
		file, err := webfs.Find(path.Clean("/" + mux.Vars(req)["path"]))
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
		file, err := webfs.Find(path.Clean("/" + mux.Vars(req)["path"]))
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
	if build == "debug" {
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
