package server

import (
	"html/template"
	"net/http"
	"strings"
)

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign in · remote-tape</title></head>
<body>
  <main>
    <h1>Sign in</h1>
    {{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
    <form method="post" action="/login">
      <input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
      <label>Password <input name="password" type="password" autocomplete="current-password" autofocus required></label>
      <button type="submit">Sign in</button>
    </form>
  </main>
</body>
</html>`))

type loginPageData struct {
	CSRFToken string
	Error     string
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.renderLogin(w, r, "")
	case http.MethodPost:
		if s.auth == nil {
			writeError(w, http.StatusServiceUnavailable, "authentication unavailable")
			return
		}
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "form body is invalid")
			return
		}
		if err := s.auth.Login(w, r, r.FormValue("password")); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid password")
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if s.auth != nil {
		s.auth.ClearSessionCookie(w)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) apiAuthSession(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if s.auth == nil {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false, "subject": "", "csrf_token": ""})
		return
	}
	csrf, err := s.auth.CSRFToken(w, r)
	if err != nil {
		writeOperationError(w, http.StatusInternalServerError, "issue csrf", err)
		return
	}
	sess, err := s.auth.Session(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false, "subject": "", "csrf_token": csrf})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "subject": sess.Subject, "csrf_token": csrf})
}

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, message string) {
	csrf := ""
	if s.auth != nil {
		var err error
		csrf, err = s.auth.CSRFToken(w, r)
		if err != nil {
			writeOperationError(w, http.StatusInternalServerError, "issue csrf", err)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginTemplate.Execute(w, loginPageData{CSRFToken: csrf, Error: message})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	if s.auth == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if protectedPath(r.URL.Path) {
			if _, err := s.auth.Session(r); err != nil {
				if isAPIPath(r.URL.Path) {
					writeError(w, http.StatusUnauthorized, "authentication required")
					return
				}
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	if s.auth == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresCSRF(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if err := s.auth.CheckCSRF(r); err != nil {
			writeError(w, http.StatusForbidden, "csrf token required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func publicPath(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/login" || path == "/api/auth/session" || path == "/api/join" || strings.HasPrefix(path, "/api/join/")
}

func protectedPath(path string) bool {
	return path == "/" || path == "/api/sessions" || strings.HasPrefix(path, "/api/sessions/") || !isAPIPath(path)
}

func requiresCSRF(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	if path == "/login" || path == "/logout" || path == "/api/sessions" {
		return true
	}
	if !strings.HasPrefix(path, "/api/sessions/") {
		return false
	}
	return strings.HasSuffix(path, "/start") || strings.HasSuffix(path, "/end") || strings.HasSuffix(path, "/confirm-download") || strings.HasSuffix(path, "/retry")
}
