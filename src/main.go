package main

import (
	"bytes"
	"context"
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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
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
	staticAssets  = genStaticAssets()
)

type contextKey int

const (
	pathContextKey = iota + 1
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
	urlRoot := flag.String("urlroot", "", "The HTTP root, must not end with a slash")
	piwikRoot := flag.String("piwik-root", "", "The HTTP root of a Piwik installation, must not end with a slash")
	piwikSiteID := flag.Int("piwik-site", 0, "The Piwik Site ID")
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

	if *urlRoot == "" {
		*urlRoot = fmt.Sprintf("http://%s", *listenAddress)
	}

	var thumbCache cache.Cache
	var sessionBaseDir string
	if *cacheDir != "" {
		d, err := resolveHome(filepath.Join(*cacheDir, "thumbs"))
		if err != nil {
			log.Fatal(err)
		}
		cache, err := filecache.NewCache(d, 0)
		if err != nil {
			log.Fatal(err)
		}
		thumbCache = cache
		sessionBaseDir = *cacheDir
	} else {
		thumbCache = memcache.NewCache()
		sessionBaseDir = os.TempDir()
	}

	mount, err := resolveHome(strings.TrimSuffix(*mountPath, "/"))
	if err != nil {
		log.Fatal(err)
	}
	filesystem, err := fs.NewFilesystem(mount, thumbCache)
	if err != nil {
		log.Fatal(err)
	}

	var authenticator Authenticator
	if *noPasswd {
		authenticator = NilAuthenticator{Filesystem: filesystem}
		log.Println("Password authentication disabled")
	} else {
		d, err := resolveHome(filepath.Join(sessionBaseDir, "sessions"))
		if err != nil {
			log.Fatal(err)
		}
		auth, err := NewBasicAuthenticator(filesystem, d)
		if err != nil {
			log.Fatal(err)
		}
		authenticator = auth
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	for _, file := range assets.AssetNames() {
		if !strings.HasPrefix(file, PUBLIC) {
			continue
		}
		urlPath := strings.TrimPrefix(file, PUBLIC)
		r.Mount(urlPath, AssetServeHandler(file))
	}

	web := Web{
		fs:            filesystem,
		thumbCache:    thumbCache,
		authenticator: authenticator,
		urlRoot:       *urlRoot,
		piwikRoot:     *piwikRoot,
		piwikSiteID:   *piwikSiteID,
	}

	r.Group(func(r chi.Router) {
		r.Use(fsPathCtx)
		r.Get("/view/*", web.view)
		r.Get("/thumb/*", web.thumb)
		r.Get("/get/*", web.download)
		r.Get("/download/*", web.downloadZip)
	})

	if *pregenThumbs {
		go filesystem.PregenerateThumbnails(THUMB_WIDTH, THUMB_HEIGHT)
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

func fsPathCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawPath := filepath.Clean("/" + chi.URLParam(r, "*"))
		path, err := url.PathUnescape(rawPath)
		if err != nil {
			w.Write([]byte(err.Error()))
			return
		}
		ctx := context.WithValue(r.Context(), pathContextKey, path)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type Web struct {
	fs            *fs.Filesystem
	authenticator Authenticator
	thumbCache    cache.Cache

	urlRoot     string
	piwikRoot   string
	piwikSiteID int
}

func (web *Web) view(w http.ResponseWriter, r *http.Request) {
	renderFile := func(path string) {
		fileI, err := web.fs.View(path, web.authenticator.FSAuthenticator(r))
		if err != nil {
			log.Printf("Could not view %q: %v", path, err)
			return
		}

		if files, ok := fileI.([]fs.File); ok {
			tmplFiles := make([]map[string]interface{}, len(files))
			for i, child := range files {
				err := web.authenticator.FSAuthenticator(r).IsAuthenticated(child.Path)
				if err != nil && err != fs.ErrNeedAuthentication {
					log.Printf("Could not check whether %q is authenticated: %v", child.Name(), err)
				}
				isUnlocked := err == nil

				tmplFiles[i] = map[string]interface{}{
					"name": child.Name(),
					"path": child.RelPath,
					"type": func() string {
						if child.Info.IsDir() {
							return "directory"
						}
						mime, err := thumb.MimeType(child.Path)
						if err != nil {
							panic(err)
						}
						return mime
					}(),
					"hasThumb": func() bool {
						if !isUnlocked {
							ok, _ := directoryth.HasIconThumb(child.Path)
							return ok
						}
						th, _ := thumb.FindThumber(child.Path)
						return th != nil
					}(),
					"hasPassword": func() bool {
						hasPassword, err := web.authenticator.HasPassword(child.Path)
						if err != nil {
							log.Printf("Could not check whether %q is protected: %v", path, err)
							return true
						}
						return hasPassword
					}(),
					"isUnlocked": isUnlocked,
				}
			}

			args := web.baseTeplateArgs()
			args["files"] = tmplFiles
			args["fs"] = web.fs
			args["path"] = path
			args["title"] = filepath.Base(path)
			if err := getPageTemplate("main.html").Execute(w, args); err != nil {
				panic(err)
			}
			return
		}

		file := fileI.(fs.File)

		// Scale down the image to reduce transfer time to the client.
		if ok, err := thumb.AcceptMimes(file.Path, "image/jpeg", "image/png"); err != nil {
			log.Println(err)
			return
		} else if ok {
			const WIDTH, HEIGHT = 1366, 768
			cachedImage, modTime, err := cache.CacheFile(web.thumbCache, file.Path, "view", func(filename string, wr io.Writer) error {
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
			return
		}

		http.ServeFile(w, r, file.Path)
	}

	path := r.Context().Value(pathContextKey).(string)

	if ok, err := web.authenticator.Authenticate(web.fs.RealPath(path), w, r); err != nil {
		log.Println(err)
		return
	} else if !ok {
		parent, err := web.fs.FirstAccessibleParent(path, web.authenticator.FSAuthenticator(r))
		if err == fs.ErrNeedAuthentication {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		} else if err != nil {
			log.Printf("Could not get first accessible parent of %q: %v", path, err)
			return
		}
		renderFile(parent)
		return
	}

	renderFile(path)
}

func (web *Web) thumb(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.Context().Value(pathContextKey).(string), ".jpg")

	img, mime, modTime, err := web.fs.Thumbnail(path, THUMB_WIDTH, THUMB_HEIGHT, web.authenticator.FSAuthenticator(r))
	if err == fs.ErrFileDoesNotExist {
		http.NotFound(w, r)
		return
	} else if err == fs.ErrNeedAuthentication {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	} else if err == fs.ErrNoThumbnail {
		http.NotFound(w, r)
		return
	} else if err != nil {
		log.Printf("Could not get thumbnail for %q: %v", path, err)
		return
	}
	defer img.Close()

	w.Header().Set("Content-Type", mime)
	http.ServeContent(w, r, filepath.Base(path)+".jpg", modTime, img)
}

func (web *Web) download(w http.ResponseWriter, r *http.Request) {
	path := r.Context().Value(pathContextKey).(string)
	filepath, err := web.fs.Filepath(path, web.authenticator.FSAuthenticator(r))
	if err == fs.ErrFileDoesNotExist {
		http.NotFound(w, r)
		return
	} else if err == fs.ErrNeedAuthentication {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	} else if err != nil {
		log.Printf("Could not get file path for %q: %v", path, err)
		return
	}

	http.ServeFile(w, r, filepath)
}

func (web *Web) downloadZip(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.Context().Value(pathContextKey).(string), ".zip")

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"webfs.zip\"")

	err := web.fs.Zip(path, w, web.authenticator.FSAuthenticator(r))
	if err == fs.ErrFileDoesNotExist {
		http.NotFound(w, r)
		return
	} else if err == fs.ErrNeedAuthentication {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	} else if err != nil {
		log.Printf("Could not zip %q: %v", path, err)
		return
	}
}

func (web *Web) baseTeplateArgs() map[string]interface{} {
	return map[string]interface{}{
		"build":       build,
		"version":     version,
		"versionDate": versionDate,

		"urlroot": web.urlRoot,
		"assets":  staticAssets,
		"time":    time.Now(),

		"piwik":       web.piwikRoot != "" && web.piwikSiteID != 0,
		"piwikRoot":   web.piwikRoot,
		"piwikSiteID": web.piwikSiteID,
	}
}

func getPageTemplate(name string) *template.Template {
	if build == "debug" {
		return template.Must(template.New(name).Parse(string(assets.MustAsset(name))))
	}

	if tmpl, ok := pageTemplates[name]; ok {
		return tmpl
	}
	pageTemplates[name] = template.Must(template.New(name).
		Parse(string(assets.MustAsset(name))))
	return pageTemplates[name]
}

func resolveHome(p string) (string, error) {
	if len(p) == 0 || p[0] != '~' {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p, err
	}
	return filepath.Join(home, p[1:]), nil
}
