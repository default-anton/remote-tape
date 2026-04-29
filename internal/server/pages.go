package server

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const (
	controlIndexFile = "index.control.html"
	authIndexFile    = "index.auth.html"
	joinIndexFile    = "index.join.html"
)

func (s *Server) app(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}

	if isAPIPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if isAuthAssetPath(r.URL.Path) {
		s.serveAuthAsset(w, r)
		return
	}
	if isJoinAssetPath(r.URL.Path) {
		s.serveJoinAsset(w, r)
		return
	}
	if isJoinPath(r.URL.Path) {
		s.serveJoinIndex(w, r)
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

func isJoinPath(urlPath string) bool {
	return urlPath == "/join" || strings.HasPrefix(urlPath, "/join/")
}

func isAuthAssetPath(urlPath string) bool {
	return strings.HasPrefix(urlPath, "/auth-assets/")
}

func isJoinAssetPath(urlPath string) bool {
	return strings.HasPrefix(urlPath, "/join-assets/")
}

func isAPIPath(urlPath string) bool {
	return urlPath == "/api" || strings.HasPrefix(urlPath, "/api/")
}

func (s *Server) serveControlIndex(w http.ResponseWriter, r *http.Request) {
	s.serveIndex(w, r, s.controlUI, controlIndexFile, "control UI is not built; run pnpm --dir web build:control before building the Go binary", "control UI unavailable")
}

func (s *Server) serveAuthIndex(w http.ResponseWriter, r *http.Request) {
	s.serveIndex(w, r, s.authUI, authIndexFile, "auth UI is not built; run pnpm --dir web build:auth before building the Go binary", "auth UI unavailable")
}

func (s *Server) serveJoinIndex(w http.ResponseWriter, r *http.Request) {
	s.serveIndex(w, r, s.joinUI, joinIndexFile, "join UI is not built; run pnpm --dir web build:join before building the Go binary", "join UI unavailable")
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

func (s *Server) serveAuthAsset(w http.ResponseWriter, r *http.Request) {
	assetPath, ok := prefixedStaticFilePath("/auth-assets/", r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, r, s.authUI, assetPath, "auth UI asset unavailable")
}

func (s *Server) serveJoinAsset(w http.ResponseWriter, r *http.Request) {
	assetPath, ok := prefixedStaticFilePath("/join-assets/", r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, r, s.joinUI, assetPath, "join UI asset unavailable")
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

func prefixedStaticFilePath(prefix string, urlPath string) (string, bool) {
	if !strings.HasPrefix(urlPath, prefix) {
		return "", false
	}
	assetPath := strings.TrimPrefix(urlPath, prefix)
	if !fs.ValidPath(assetPath) || assetPath == "." {
		return "", false
	}
	return assetPath, isAssetPath("/" + assetPath)
}
