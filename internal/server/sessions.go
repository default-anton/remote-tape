package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

type listSessionEventsResponse struct {
	Events     []session.Event         `json:"events"`
	Pagination sessionEventsPagination `json:"pagination"`
}

type sessionEventsPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type sessionEventExportLine struct {
	ID           int64   `json:"id"`
	SessionID    string  `json:"session_id"`
	Type         string  `json:"type"`
	Message      *string `json:"message"`
	Metadata     any     `json:"metadata"`
	MetadataJSON *string `json:"metadata_json,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

type listSessionsResponse struct {
	Sessions            []session.Session   `json:"sessions"`
	Pagination          sessionsPagination  `json:"pagination"`
	Summary             sessionsSummary     `json:"summary"`
	Filters             sessionsFilters     `json:"filters"`
	HasPollable         bool                `json:"has_pollable"`
	ProvisioningOptions provisioningOptions `json:"provisioning_options"`
}

type sessionsPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type sessionsSummary struct {
	Total                  int `json:"total"`
	Provisioning           int `json:"provisioning"`
	Ready                  int `json:"ready"`
	Active                 int `json:"active"`
	AwaitingManualDownload int `json:"awaiting_manual_download"`
	Failed                 int `json:"failed"`
}

type sessionsFilters struct {
	Statuses []sessionFilterOption `json:"statuses"`
	Regions  []sessionFilterOption `json:"regions"`
}

type sessionFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func (s *Server) apiSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.repo.ListSessions(r.Context(), listSessionsInputFromQuery(r.URL.Query()))
		if err != nil {
			writeOperationError(w, http.StatusInternalServerError, "list sessions", err)
			return
		}
		writeJSON(w, http.StatusOK, listSessionsResponseFor(result, s.options.Environment, s.options.DefaultRegion, s.options.DefaultInstanceSize))
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

func (s *Server) apiSessionSlug(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/api/session-slugs/")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}
	availability, err := s.repo.CheckSlugAvailability(r.Context(), slug)
	if err != nil {
		writeOperationError(w, http.StatusInternalServerError, "check slug availability", err)
		return
	}
	writeJSON(w, http.StatusOK, availability)
}

func (s *Server) apiSession(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 {
		switch parts[1] {
		case "force-destroy":
			s.apiForceDestroySession(w, r, parts[0])
			return
		case "events":
			s.apiSessionEvents(w, r, parts[0])
			return
		case "events.jsonl":
			s.apiSessionEventsJSONL(w, r, parts[0])
			return
		}
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

func (s *Server) apiSessionEvents(w http.ResponseWriter, r *http.Request, id string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if id == "" {
		http.NotFound(w, r)
		return
	}
	result, err := s.repo.ListSessionEvents(r.Context(), id, session.ListSessionEventsInput{
		Page:     parsePositiveInt(r.URL.Query().Get("page")),
		PageSize: parsePositiveInt(r.URL.Query().Get("page_size")),
	})
	if errors.Is(err, session.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		writeOperationError(w, http.StatusInternalServerError, "list session events", err)
		return
	}
	writeJSON(w, http.StatusOK, listSessionEventsResponse{Events: result.Events, Pagination: sessionEventsPaginationFor(result)})
}

func (s *Server) apiSessionEventsJSONL(w http.ResponseWriter, r *http.Request, id string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if id == "" {
		http.NotFound(w, r)
		return
	}
	headersWritten := false
	writeHeaders := func() {
		if headersWritten {
			return
		}
		w.Header().Set("Content-Type", "application/jsonl")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="remote-tape-session-%s-events.jsonl"`, contentDispositionFilenamePart(id)))
		w.WriteHeader(http.StatusOK)
		headersWritten = true
	}
	encoder := json.NewEncoder(w)
	err := s.repo.StreamSessionEvents(r.Context(), id, func(event session.Event) error {
		writeHeaders()
		return encoder.Encode(sessionEventExportLineFor(event))
	})
	if errors.Is(err, session.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		if !headersWritten {
			writeOperationError(w, http.StatusInternalServerError, "export session events", err)
		}
		return
	}
	writeHeaders()
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

func listSessionsInputFromQuery(values url.Values) session.ListSessionsInput {
	return session.ListSessionsInput{
		Page:      parsePositiveInt(values.Get("page")),
		PageSize:  parsePositiveInt(values.Get("page_size")),
		Sort:      values.Get("sort"),
		Direction: values.Get("direction"),
		Statuses:  values["status"],
		Regions:   values["region"],
		Query:     values.Get("q"),
	}
}

func parsePositiveInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return 0
	}
	return parsed
}

func listSessionsResponseFor(result session.ListSessionsResult, environment string, defaultRegion string, defaultSize string) listSessionsResponse {
	return listSessionsResponse{
		Sessions:            result.Sessions,
		Pagination:          sessionsPaginationFor(result),
		Summary:             sessionsSummaryFor(result),
		Filters:             sessionsFiltersFor(),
		HasPollable:         result.PollableCount > 0,
		ProvisioningOptions: provisioningOptionsFor(environment, defaultRegion, defaultSize),
	}
}

func sessionsPaginationFor(result session.ListSessionsResult) sessionsPagination {
	totalPages := 0
	if result.Total > 0 {
		totalPages = (result.Total + result.PageSize - 1) / result.PageSize
	}
	return sessionsPagination{Page: result.Page, PageSize: result.PageSize, Total: result.Total, TotalPages: totalPages}
}

func sessionEventsPaginationFor(result session.ListSessionEventsResult) sessionEventsPagination {
	totalPages := 0
	if result.Total > 0 {
		totalPages = (result.Total + result.PageSize - 1) / result.PageSize
	}
	return sessionEventsPagination{Page: result.Page, PageSize: result.PageSize, Total: result.Total, TotalPages: totalPages}
}

func sessionEventExportLineFor(event session.Event) sessionEventExportLine {
	line := sessionEventExportLine{
		ID:        event.ID,
		SessionID: event.SessionID,
		Type:      event.Type,
		Message:   event.Message,
		Metadata:  nil,
		CreatedAt: event.CreatedAt,
	}
	if event.MetadataJSON == nil {
		return line
	}
	var metadata any
	if err := json.Unmarshal([]byte(*event.MetadataJSON), &metadata); err != nil {
		line.MetadataJSON = event.MetadataJSON
		return line
	}
	line.Metadata = metadata
	return line
}

func contentDispositionFilenamePart(value string) string {
	value = strings.NewReplacer(`"`, "-", "\\", "-", "/", "-").Replace(value)
	if value == "" {
		return "session"
	}
	return value
}

func sessionsSummaryFor(result session.ListSessionsResult) sessionsSummary {
	counts := result.StatusCounts
	return sessionsSummary{
		Total:                  result.Total,
		Provisioning:           counts["created"] + counts["provisioning"] + counts["waiting_for_dns"],
		Ready:                  counts["ready"],
		Active:                 counts["active"],
		AwaitingManualDownload: counts["finalizing"] + counts["awaiting_manual_download"] + counts["teardown_pending"],
		Failed:                 result.AttentionCount,
	}
}

func sessionsFiltersFor() sessionsFilters {
	regions := make([]sessionFilterOption, 0, len(provisioningCatalog.Regions))
	for _, region := range provisioningCatalog.Regions {
		regions = append(regions, sessionFilterOption{Value: region.Slug, Label: region.Label})
	}
	return sessionsFilters{Statuses: sessionStatusFilterOptions(), Regions: regions}
}

func sessionStatusFilterOptions() []sessionFilterOption {
	return []sessionFilterOption{
		{Value: "created", Label: "Created"},
		{Value: "provisioning", Label: "Provisioning"},
		{Value: "waiting_for_dns", Label: "Waiting for DNS"},
		{Value: "ready", Label: "Ready"},
		{Value: "active", Label: "Active"},
		{Value: "finalizing", Label: "Finalizing"},
		{Value: "awaiting_manual_download", Label: "Awaiting manual download"},
		{Value: "teardown_pending", Label: "Teardown pending"},
		{Value: "tearing_down", Label: "Tearing down"},
		{Value: "ended", Label: "Ended"},
		{Value: "failed", Label: "Failed"},
	}
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
	if err := validateProvisioningSelection(s.options.Environment, region, size); err != nil {
		return session.CreateResult{}, err
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
	case errors.Is(err, errInvalidProvisioningSelection):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, session.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, session.ErrSlugConflicts):
		writeError(w, http.StatusConflict, "session slug already exists")
	default:
		writeOperationError(w, http.StatusInternalServerError, "create session", err)
	}
}
