package server

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const controlIndexFile = "index.control.html"

func (s *Server) app(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}

	if isAPIPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if isAssetPath(r.URL.Path) {
		s.serveControlAsset(w, r)
		return
	}
	s.serveControlIndex(w, r)
}

func isAssetPath(urlPath string) bool {
	return strings.HasPrefix(urlPath, "/assets/") || strings.HasPrefix(urlPath, "/favicon") || path.Ext(urlPath) != ""
}

func isAPIPath(urlPath string) bool {
	return urlPath == "/api" || strings.HasPrefix(urlPath, "/api/")
}

func (s *Server) serveControlIndex(w http.ResponseWriter, r *http.Request) {
	s.serveIndex(w, r, s.controlUI, controlIndexFile, "control UI is not built; run pnpm --dir web build:control before building the Go binary", "control UI unavailable")
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request, fileSystem fs.FS, file string, missingMessage string, errorMessage string) {
	if _, err := fs.Stat(fileSystem, file); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.Error(w, missingMessage, http.StatusServiceUnavailable)
			return
		}
		http.Error(w, errorMessage, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, fileSystem, file)
}

func (s *Server) serveControlAsset(w http.ResponseWriter, r *http.Request) {
	assetPath, ok := staticFilePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, r, s.controlUI, assetPath, "control UI asset unavailable")
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request, fileSystem fs.FS, assetPath string, errorMessage string) {
	if _, err := fs.Stat(fileSystem, assetPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, errorMessage, http.StatusInternalServerError)
		return
	}
	if strings.HasPrefix(assetPath, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	http.ServeFileFS(w, r, fileSystem, assetPath)
}

func staticFilePath(urlPath string) (string, bool) {
	assetPath := strings.TrimPrefix(urlPath, "/")
	if !fs.ValidPath(assetPath) || assetPath == "." {
		return "", false
	}
	return assetPath, isAssetPath("/" + assetPath)
}
