package server

import (
	"bytes"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"hfbridge/static"
)

var (
	staticRoot fs.FS
)

func init() {
	staticRoot = static.Root()
}

func serveStaticIfExists(c *gin.Context) bool {
	if staticRoot == nil {
		return false
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		return false
	}

	reqPath := strings.TrimPrefix(c.Request.URL.Path, "/")
	if reqPath == "" {
		reqPath = "index.html"
	}

	for _, candidate := range candidatePaths(reqPath) {
		if serveCandidate(c, candidate) {
			return true
		}
	}
	return false
}

func serveOptimizedImage(c *gin.Context) bool {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		return false
	}

	rawURL := c.Query("url")
	if rawURL == "" || !strings.HasPrefix(rawURL, "/") || strings.Contains(rawURL, "..") {
		return false
	}

	target := strings.TrimPrefix(rawURL, "/")
	return serveCandidate(c, target)
}

func serveCandidate(c *gin.Context, name string) bool {
	file, err := staticRoot.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	r2 := c.Request.Clone(c.Request.Context())
	r2.URL.Path = "/" + name
	if ext := path.Ext(name); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			c.Header("Content-Type", ct)
		}
	}

	var reader io.ReadSeeker
	if rs, ok := file.(io.ReadSeeker); ok {
		reader = rs
	} else {
		data, err := io.ReadAll(file)
		if err != nil {
			return false
		}
		reader = bytes.NewReader(data)
	}

	http.ServeContent(c.Writer, r2, path.Base(name), info.ModTime(), reader)
	return true
}

func candidatePaths(p string) []string {
	var candidates []string
	trimmed := strings.TrimSuffix(p, "/")
	if trimmed != p {
		candidates = append(candidates, p, trimmed+"/index.html")
	} else {
		candidates = append(candidates, p, p+"/index.html")
		if !strings.HasSuffix(p, ".html") {
			candidates = append(candidates, p+".html")
		}
	}
	return dedupe(candidates)
}

func dedupe(items []string) []string {
	if len(items) <= 1 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
