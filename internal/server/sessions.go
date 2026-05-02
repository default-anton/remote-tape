package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/default-anton/remote-tape/internal/session"
)

type createSessionRequest struct {
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	InstanceRegion string `json:"instance_region"`
	InstanceSize   string `json:"instance_size"`
	ImageID        string `json:"image_id"`
}

type createSessionResponse struct {
	Session   session.Session      `json:"session"`
	JoinLinks map[string]joinLink  `json:"join_links"`
	Events    []session.Event      `json:"events"`
	Tokens    map[string]tokenInfo `json:"tokens"`
}

type tokenInfo struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type joinLink struct {
	URL  string `json:"url"`
	Role string `json:"role"`
}

type joinSessionResponse struct {
	Session joinSessionInfo `json:"session"`
	Token   joinTokenInfo   `json:"token"`
}

type joinSessionInfo struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type joinTokenInfo struct {
	Role string `json:"role"`
}

type forceDestroyRequest struct {
	Confirmation string `json:"confirmation"`
}

func (s *Server) apiSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions, err := s.repo.ListSessions(r.Context())
		if err != nil {
			writeOperationError(w, http.StatusInternalServerError, "list sessions", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	case http.MethodPost:
		var req createSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}
		result, err := s.createSession(r, req)
		if err != nil {
			s.writeSessionError(w, err)
			return
		}
		detail, err := s.repo.GetSession(r.Context(), result.Session.ID)
		if err != nil {
			writeOperationError(w, http.StatusInternalServerError, "load created session", err)
			return
		}
		writeJSON(w, http.StatusCreated, s.createResponse(result, detail.Events))
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) apiSession(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "force-destroy" {
		s.apiForceDestroySession(w, r, parts[0])
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	id := path
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	detail, err := s.repo.GetSession(r.Context(), id)
	if errors.Is(err, session.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		writeOperationError(w, http.StatusInternalServerError, "get session", err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) apiForceDestroySession(w http.ResponseWriter, r *http.Request, id string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if id == "" {
		http.NotFound(w, r)
		return
	}
	var req forceDestroyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}
	detail, err := s.repo.GetSession(r.Context(), id)
	if errors.Is(err, session.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		writeOperationError(w, http.StatusInternalServerError, "get session", err)
		return
	}
	if !forceDestroyEligible(detail.Session.Status) {
		writeError(w, http.StatusConflict, "session server can be force destroyed only while provisioning, waiting for DNS, or failed")
		return
	}
	expected := "destroy " + detail.Session.Slug
	if strings.TrimSpace(req.Confirmation) != expected {
		writeError(w, http.StatusBadRequest, "confirmation must exactly match: "+expected)
		return
	}
	opCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
	defer cancel()
	changed, err := s.repo.MarkForceDestroyStarted(opCtx, detail.Session.ID)
	if err != nil {
		writeOperationError(w, http.StatusInternalServerError, "mark force destroy started", err)
		return
	}
	if !changed {
		writeError(w, http.StatusConflict, "session is no longer eligible for force destroy")
		return
	}
	detail, err = s.repo.GetSession(opCtx, id)
	if err != nil {
		writeOperationError(w, http.StatusInternalServerError, "get session", err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func forceDestroyEligible(status string) bool {
	return status == "provisioning" || status == "waiting_for_dns" || status == "failed"
}

func (s *Server) apiJoin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/join/"), "/")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}
	result, err := s.repo.JoinSession(r.Context(), slug, r.URL.Query().Get("token"))
	if errors.Is(err, session.ErrInvalidToken) {
		writeError(w, http.StatusNotFound, "join link not found")
		return
	}
	if err != nil {
		writeOperationError(w, http.StatusInternalServerError, "join session", err)
		return
	}
	writeJSON(w, http.StatusOK, joinResponse(result))
}

func joinResponse(result session.JoinResult) joinSessionResponse {
	return joinSessionResponse{
		Session: joinSessionInfo{
			Slug:   result.Session.Slug,
			Title:  result.Session.Title,
			Status: result.Session.Status,
		},
		Token: joinTokenInfo{Role: result.Token.Role},
	}
}

func (s *Server) createSession(r *http.Request, req createSessionRequest) (session.CreateResult, error) {
	region := strings.TrimSpace(req.InstanceRegion)
	if region == "" {
		region = s.options.DefaultRegion
	}
	size := strings.TrimSpace(req.InstanceSize)
	if size == "" {
		size = s.options.DefaultInstanceSize
	}
	imageID := strings.TrimSpace(req.ImageID)
	if imageID == "" {
		imageID = s.options.ImageID
	}
	return s.repo.CreateSession(r.Context(), session.CreateInput{
		Title:              req.Title,
		Slug:               req.Slug,
		InstanceRegion:     region,
		InstanceSize:       size,
		ImageID:            imageID,
		SessionsBaseDomain: s.options.SessionsBaseDomain,
	})
}

func (s *Server) createResponse(result session.CreateResult, events []session.Event) createSessionResponse {
	hostURL := s.joinURL(result.Session.Slug, result.HostToken)
	guestURL := s.joinURL(result.Session.Slug, result.GuestToken)
	return createSessionResponse{
		Session: result.Session,
		JoinLinks: map[string]joinLink{
			"host":  {URL: hostURL, Role: "host"},
			"guest": {URL: guestURL, Role: "guest"},
		},
		Events: events,
		Tokens: map[string]tokenInfo{
			"host":  {ID: result.HostTokenID, Token: result.HostToken},
			"guest": {ID: result.GuestTokenID, Token: result.GuestToken},
		},
	}
}

func (s *Server) joinURL(slug string, token string) string {
	base := strings.TrimRight(s.options.ControlPlaneURL, "/")
	return fmt.Sprintf("%s/join/%s?token=%s", base, url.PathEscape(slug), url.QueryEscape(token))
}

func (s *Server) writeSessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, session.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, session.ErrSlugConflicts):
		writeError(w, http.StatusConflict, "session slug already exists")
	default:
		writeOperationError(w, http.StatusInternalServerError, "create session", err)
	}
}
