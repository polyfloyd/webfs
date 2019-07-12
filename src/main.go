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
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi"
	"github.com/nfnt/resize"

	"webfs/src/assets"
	"webfs/src/cache"
	"webfs/src/cache/filecache"
	"webfs/src/cache/memcache"
	"webfs/src/fs"
	"webfs/src/thumb"
	directoryth "webfs/src/thumb/directory"
	_ "webfs/src/thumb/image"
	_ "webfs/src/thumb/vector"
	_ "webfs/src/thumb/video"
)

const (
	PUBLIC = "public"
)

const (
	THUMB_WIDTH  = 140
	THUMB_HEIGHT = 140
)

var (
	build       = "<unset>"
	version     = "<unset>"
	versionDate = "<unset>"
)

// Global vars are bad, but these are not supposed to be changed.
var (
	startTime     = time.Now()
	pageTemplates = map[string]*template.Template{}
	staticAssets  map[string][]string
)

var (
	urlRoot     string
	piwikRoot   string
	piwikSiteID int
)

type AssetServeHandler string

func (name AssetServeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(string(name))))
	modTime := startTime
	if build == "debug" {
		modTime = time.Now()
	}
	http.ServeContent(w, r, string(name), modTime, bytes.NewReader(assets.MustAsset(string(name))))
}

func main() {
	log.Printf("Version: %v (%v)\n", version, build)

	listenAddress := flag.String("listen", "localhost:8080", "The HTTP root of a Piwik installation, must not end with a slash")
	flag.StringVar(&urlRoot, "urlroot", "", "The HTTP root, must not end with a slash")
	flag.StringVar(&piwikRoot, "piwik-root", "", "The HTTP root of a Piwik installation, must not end with a slash")
	flag.IntVar(&piwikSiteID, "piwik-site", 0, "The Piwik Site ID")
	pregenThumbs := flag.Bool("pregen-thumbs", false, "Generate thumbnails for every file in all configured filesystems on startup")
	defaultCacheDir := filepath.Join(os.TempDir(), fmt.Sprintf("webfs-%d", os.Getuid()))
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

	var thumbCache cache.Cache
	var sessionBaseDir string
	if *cacheDir != "" {
		cache, err := filecache.NewCache(fs.ResolveHome(filepath.Join(*cacheDir, "thumbs")), 0)
		if err != nil {
			log.Fatal(err)
		}
		thumbCache = cache
		sessionBaseDir = *cacheDir
	} else {
		thumbCache = memcache.NewCache()
		sessionBaseDir = os.TempDir()
	}

	var authenticator Authenticator
	if *noPasswd {
		authenticator = NilAuthenticator{}
		log.Println("Password authentication disabled")
	} else {
		auth, err := NewBasicAuthenticator(filepath.Join(sessionBaseDir, "sessions"))
		if err != nil {
			log.Fatal(err)
		}
		authenticator = auth
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("%s %s", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	})

	for _, file := range assets.AssetNames() {
		if !strings.HasPrefix(file, PUBLIC) {
			continue
		}
		urlPath := strings.TrimPrefix(file, PUBLIC)
		r.Mount(urlPath, AssetServeHandler(file))
	}

	filesystem, err := fs.NewFilesystem(strings.TrimSuffix(*mountPath, "/"))
	if err != nil {
		log.Fatal(err)
	}

	web := Web{
		fs:            filesystem,
		thumbCache:    thumbCache,
		authenticator: authenticator,
	}

	r.Get("/view/*", web.view)
	r.Get("/thumb/*", web.thumb)
	r.Get("/get/*", web.get)
	r.Get("/download/*", web.download)

	if *pregenThumbs {
		go pregenerateThumbnails(filesystem, thumbCache)
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

func pregenerateThumbnails(filesystem *fs.Filesystem, thumbCache cache.Cache) {
	numRunners := runtime.NumCPU() / 2
	if numRunners <= 0 {
		numRunners = 1
	}
	log.Printf("Generating thumbnails using %v workers", numRunners)

	fileStream := make(chan *fs.File)
	go func() {
		defer close(fileStream)
		log.Printf("Generating thumbs")
		filepath.Walk(filesystem.RealPath, func(path string, info os.FileInfo, err error) error {
			if path == filesystem.RealPath || fs.IsDotFile(path) {
				return nil
			}
			fileStream <- &fs.File{
				Info: info,
				Path: strings.TrimPrefix(path, filesystem.RealPath+"/"),
				Fs:   filesystem,
			}
			return nil
		})
	}()

	var wg sync.WaitGroup
	wg.Add(numRunners)
	for i := 0; i < numRunners; i++ {
		go func() {
			defer wg.Done()
			for file := range fileStream {
				log.Println(file.Path)
				if thumb, _, err := thumb.ThumbFile(thumbCache, file.RealPath(), THUMB_WIDTH, THUMB_HEIGHT); err != nil {
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

		switch filepath.Ext(file) {
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

type Web struct {
	fs            *fs.Filesystem
	authenticator Authenticator
	thumbCache    cache.Cache
}

func (web *Web) view(w http.ResponseWriter, r *http.Request) {
	var renderFile func(*fs.File)
	renderFile = func(file *fs.File) {
		auth, err := web.authenticator.Authenticate(file, w, r)
		if err != nil {
			panic(err)
		}
		if !auth {
			if parent := file.Parent(); parent != nil {
				renderFile(parent)
			} else {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			}
			return
		}

		if !file.Info.IsDir() {
			if ok, err := thumb.AcceptMimes(file.RealPath(), "image/jpeg", "image/png"); err != nil {
				panic(err)
			} else if ok {
				// Scale down the image to reduce transfer time to the client.
				const WIDTH, HEIGHT = 1366, 768
				cachedImage, modTime, err := cache.CacheFile(web.thumbCache, file.RealPath(), "view", func(filename string, wr io.Writer) error {
					fd, err := os.Open(filename)
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
					http.NotFound(w, r)
					return
				}
				if cachedImage == nil {
					http.NotFound(w, r)
					return
				}
				defer cachedImage.Close()
				http.ServeContent(w, r, file.Info.Name(), modTime, cachedImage)

			} else {
				http.ServeFile(w, r, file.RealPath())
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
			isUnlocked, err := web.authenticator.IsUnlocked(child, r)
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
						mime, err := thumb.MimeType(child.RealPath())
						if err != nil {
							panic(err)
						}
						return mime
					}
				}(),
				"hasThumb": func() bool {
					if !isUnlocked {
						ok, _ := directoryth.HasIconThumb(child.RealPath())
						return ok
					}
					th, _ := thumb.FindThumber(child.RealPath())
					return th != nil
				}(),
				"hasPassword": func() bool {
					hasPassword, err := web.authenticator.HasPassword(child)
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
		args["fs"] = web.fs
		args["path"] = file.Path
		args["title"] = filepath.Base(file.Path)
		if err := getPageTemplate("main.html").Execute(w, args); err != nil {
			panic(err)
		}
	}

	path := filepath.Clean("/" + chi.URLParam(r, "*"))
	file, err := web.fs.Find(path)
	if err != nil {
		panic(err)
	}
	if file == nil || fs.IsDotFile(file.Path) {
		http.NotFound(w, r)
		return
	}

	renderFile(file)
}

func (web *Web) thumb(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(filepath.Clean("/"+chi.URLParam(r, "*")), ".jpg")
	file, err := web.fs.Find(path)
	if err != nil {
		panic(err)
	}

	if file == nil || fs.IsDotFile(file.Path) {
		http.NotFound(w, r)
		return
	}

	if auth, err := web.authenticator.IsUnlocked(file, r); err != nil {
		panic(err)
	} else if !auth {
		if ok, err := directoryth.HasIconThumb(file.RealPath()); err != nil {
			panic(err)
		} else if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	cachedThumb, modTime, err := thumb.ThumbFile(web.thumbCache, file.RealPath(), THUMB_WIDTH, THUMB_HEIGHT)
	if err != nil {
		log.Println(err)
		http.NotFound(w, r)
		return
	}
	if cachedThumb == nil {
		http.NotFound(w, r)
		return
	}
	defer cachedThumb.Close()

	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, file.Info.Name(), modTime, cachedThumb)
}

func (web *Web) get(w http.ResponseWriter, r *http.Request) {
	path := filepath.Clean("/" + chi.URLParam(r, "*"))
	file, err := web.fs.Find(path)
	if err != nil {
		panic(err)
	}
	if file == nil || fs.IsDotFile(file.Path) {
		http.NotFound(w, r)
		return
	}

	if auth, err := web.authenticator.Authenticate(file, w, r); err != nil {
		panic(err)
	} else if !auth {
		w.Write([]byte("Unauthorized"))
		return
	}

	http.ServeFile(w, r, file.RealPath())
}

func (web *Web) download(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(filepath.Clean("/"+chi.URLParam(r, "*")), ".zip")
	file, err := web.fs.Find(path)
	if err != nil {
		panic(err)
	}

	if file == nil || fs.IsDotFile(file.Path) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	if file.Path == "/" {
		w.Header().Set("Content-Disposition", "attachment; filename=\"webfs.zip\"")
	}

	filter := func(file *fs.File) (bool, error) {
		if fs.IsDotFile(file.Path) {
			return false, nil
		}
		return web.authenticator.IsUnlocked(file, r)
	}
	if err := fs.ZipTreeFilter(file, filter, w); err != nil {
		panic(err)
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
