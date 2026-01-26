package dogeboxd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/rs/cors"
)

type UIServer struct {
	mux    *http.ServeMux
	config ServerConfig
}

func serveSPA(directory string, mainIndex string) http.HandlerFunc {
	mainIndexPath := filepath.Join(directory, mainIndex)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// Block TypeScript files from being served directly
		if strings.HasSuffix(r.URL.Path, ".ts") || strings.HasSuffix(r.URL.Path, ".tsx") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = w.Write([]byte("TypeScript files cannot be served as browser modules. Build dpanel (vite build) and serve the dist/ directory, or run the Vite dev server.\n"))
			return
		}

		if r.URL.Path == "/" {
			http.ServeFile(w, r, mainIndexPath)
			return
		}

		// Turn URL path into a safe relative path for joining with directory.
		relPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if relPath == "" || relPath == "." || strings.HasPrefix(relPath, "..") {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		// In Vite builds, "publicDir" files are copied to dist root; try both dist/static/... and dist/... for /static/* requests.
		candidates := []string{relPath}
		if strings.HasPrefix(relPath, "static/") {
			candidates = append(candidates, strings.TrimPrefix(relPath, "static/"))
		}

		var filePath string
		for _, p := range candidates {
			fp := filepath.Join(directory, p)
			info, err := os.Stat(fp)
			if err == nil && !info.IsDir() {
				filePath = fp
				break
			}
		}

		if filePath == "" {
			// Can't find the requested file (or it’s a directory), serve SPA shell.
			http.ServeFile(w, r, mainIndexPath)
			return
		}

		http.ServeFile(w, r, filePath)
	}
}

func ServeUI(config ServerConfig) conductor.Service {
	entryPoint := "index.html"

	if config.Recovery {
		entryPoint = "index_recovery.html"
		log.Println("In recovery mode: Serving recovery UI")
	} else {
		log.Println("Serving normal UI")
	}

	service := UIServer{
		mux:    http.NewServeMux(),
		config: config,
	}

	service.mux.HandleFunc("/", serveSPA(config.UiDir, entryPoint))

	return service
}

func (t UIServer) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		handler := cors.AllowAll().Handler(t.mux)
		srv := &http.Server{Addr: fmt.Sprintf("%s:%d", t.config.Bind, t.config.UiPort), Handler: handler}
		go func() {
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP server public ListenAndServe: %v", err)
			}
		}()

		started <- true
		ctx := <-stop
		srv.Shutdown(ctx)
		stopped <- true
	}()
	return nil
}
