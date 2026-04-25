package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const controlIndexFile = "index.control.html"

func (s *Server) app(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}

	if isAssetPath(r.URL.Path) {
		s.serveAsset(w, r)
		return
	}
	if isControlAppRoute(r.URL.Path) {
		s.serveControlIndex(w, r)
		return
	}
	http.NotFound(w, r)
}

func isAssetPath(path string) bool {
	return strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/favicon") || filepath.Ext(path) != ""
}

func isControlAppRoute(path string) bool {
	return path == "/" || path == "/sessions" || strings.HasPrefix(path, "/sessions/") || strings.HasPrefix(path, "/join/")
}

func (s *Server) serveControlIndex(w http.ResponseWriter, r *http.Request) {
	indexPath := filepath.Join(s.options.ControlWebDistDir, controlIndexFile)
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "control UI is not built; run pnpm --dir web build", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "control UI unavailable", http.StatusInternalServerError)
		return
	}
	http.ServeFile(w, r, indexPath)
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	cleanPath := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), string(filepath.Separator))
	assetPath := filepath.Join(s.options.ControlWebDistDir, cleanPath)
	if !strings.HasPrefix(assetPath, filepath.Clean(s.options.ControlWebDistDir)+string(filepath.Separator)) && filepath.Clean(assetPath) != filepath.Clean(s.options.ControlWebDistDir) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, assetPath)
}
