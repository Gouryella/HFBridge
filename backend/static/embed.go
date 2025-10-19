package static

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed all:web
var bundled embed.FS

var (
	rootFS fs.FS
	server http.Handler
)

func init() {
	fsys, err := fs.Sub(bundled, "web")
	if err != nil {
		log.Fatalf("[static] failed to access embedded web bundle: %v", err)
	}
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		log.Fatalf("[static] embedded bundle missing index.html: %v", err)
	}

	rootFS = fsys
	server = http.FileServer(http.FS(rootFS))
}

// Root exposes the embedded static filesystem for serving/build-time access.
func Root() fs.FS {
	return rootFS
}

// Handler returns an http.Handler that can serve the embedded assets.
func Handler() http.Handler {
	return server
}
