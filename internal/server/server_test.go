package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/default-anton/remote-tape/internal/auth"
	"github.com/default-anton/remote-tape/internal/database"
)

func TestHealthz(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v", body["ok"])
	}
	if body["service"] != "remote-tape-control" {
		t.Fatalf("service = %v", body["service"])
	}
}

func TestReadyz(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v", body["ok"])
	}
	if body["database"] != "ok" {
		t.Fatalf("database = %v", body["database"])
	}
	if body["migrations_applied"] != float64(1) {
		t.Fatalf("migrations_applied = %v", body["migrations_applied"])
	}
}

func TestHealthzRejectsUnsafeMethod(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q", response.Header().Get("Allow"))
	}
}

func TestReadyzReportsDatabaseFailure(t *testing.T) {
	handler, db := newTestHandler(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != false {
		t.Fatalf("ok = %v", body["ok"])
	}
}

func TestListSessionsAPIReturnsEmptyArray(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	addCookies(request, cookies)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Sessions            []any `json:"sessions"`
		ProvisioningOptions struct {
			Defaults struct {
				Region string `json:"region"`
				Size   string `json:"size"`
			} `json:"defaults"`
			Regions                 []any               `json:"regions"`
			Sizes                   []any               `json:"sizes"`
			Availability            map[string][]string `json:"availability"`
			RecommendedSizeByRegion map[string]string   `json:"recommended_size_by_region"`
		} `json:"provisioning_options"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Sessions) != 0 {
		t.Fatalf("sessions = %+v", body.Sessions)
	}
	if body.ProvisioningOptions.Defaults.Region != "nyc3" || body.ProvisioningOptions.Defaults.Size != "s-2vcpu-4gb" {
		t.Fatalf("defaults = %+v", body.ProvisioningOptions.Defaults)
	}
	if len(body.ProvisioningOptions.Regions) == 0 || len(body.ProvisioningOptions.Sizes) == 0 || len(body.ProvisioningOptions.Availability["nyc3"]) == 0 {
		t.Fatalf("provisioning_options = %+v", body.ProvisioningOptions)
	}
	if body.ProvisioningOptions.RecommendedSizeByRegion["nyc3"] != "s-2vcpu-4gb" {
		t.Fatalf("recommended_size_by_region = %+v", body.ProvisioningOptions.RecommendedSizeByRegion)
	}
}

func TestGetSessionAPIReturnsEmptyArrays(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	if _, err := db.ExecContext(context.Background(), `
insert into sessions(id, slug, title, status, instance_region, instance_size, image_id, created_at, updated_at)
values ('sess_empty', 'empty-detail', 'Empty Detail', 'created', 'nyc3', 's-1vcpu-1gb', 'image', '2026-04-24T12:00:00.000000000Z', '2026-04-24T12:00:00.000000000Z');
`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/sessions/sess_empty", nil)
	addCookies(request, cookies)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"access_tokens":[]`) || !strings.Contains(body, `"events":[]`) {
		t.Fatalf("body = %s", body)
	}
}

func TestCreateAndGetSessionAPI(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()
	cookies, csrf := loginTestAdmin(t, handler)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"The Infra Podcast #313","slug":"the-infra-podcast-313"}`))
	addCookies(request, cookies)
	request.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var created struct {
		Session struct {
			ID         string  `json:"id"`
			Status     string  `json:"status"`
			RoomDomain *string `json:"room_domain"`
		} `json:"session"`
		JoinLinks map[string]struct {
			URL string `json:"url"`
		} `json:"join_links"`
		Events []struct {
			Type string `json:"type"`
		} `json:"events"`
		Tokens map[string]struct {
			Token string `json:"token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Session.ID == "" || created.Session.Status != "created" {
		t.Fatalf("created session = %+v", created.Session)
	}
	if created.Session.RoomDomain == nil || !strings.HasSuffix(*created.Session.RoomDomain, ".sessions.localhost") {
		t.Fatalf("room_domain = %v", created.Session.RoomDomain)
	}
	roomLabel := strings.TrimSuffix(*created.Session.RoomDomain, ".sessions.localhost")
	if !strings.HasPrefix(roomLabel, "room-") || len(roomLabel) > 63 || strings.Contains(roomLabel, "_") {
		t.Fatalf("room_domain label = %q", roomLabel)
	}
	if created.JoinLinks["host"].URL == "" || created.JoinLinks["guest"].URL == "" {
		t.Fatalf("join links = %+v", created.JoinLinks)
	}
	if created.Tokens["host"].Token == "" || created.Tokens["guest"].Token == "" {
		t.Fatalf("tokens = %+v", created.Tokens)
	}
	if len(created.Events) != 1 || created.Events[0].Type != "session.created" {
		t.Fatalf("events = %+v", created.Events)
	}

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.Session.ID, nil)
	addCookies(getReq, cookies)
	handler.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", get.Code, get.Body.String())
	}
	if strings.Contains(get.Body.String(), created.Tokens["host"].Token) || strings.Contains(get.Body.String(), created.Tokens["guest"].Token) {
		t.Fatalf("GET response leaked raw join token: %s", get.Body.String())
	}
}

func TestSlugAvailabilityAPI(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()
	cookies, csrf := loginTestAdmin(t, handler)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"Taken","slug":"taken-slug"}`))
	addCookies(createReq, cookies)
	createReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", create.Code, create.Body.String())
	}

	tests := []struct {
		name       string
		path       string
		available  bool
		valid      bool
		wantReason *string
	}{
		{name: "available", path: "/api/session-slugs/New-Slug", available: true, valid: true},
		{name: "taken", path: "/api/session-slugs/taken-slug", available: false, valid: true, wantReason: ptr("taken")},
		{name: "invalid", path: "/api/session-slugs/-bad-", available: false, valid: false, wantReason: ptr("invalid_format")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			addCookies(request, cookies)
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			var body struct {
				Available bool    `json:"available"`
				Valid     bool    `json:"valid"`
				Reason    *string `json:"reason"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Available != tt.available || body.Valid != tt.valid || !equalPtr(body.Reason, tt.wantReason) {
				t.Fatalf("body = %+v", body)
			}
		})
	}
}

func TestSlugAvailabilityAPIRequiresAuth(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/session-slugs/anything", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
}

func TestCreateSessionProvisioningValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "valid explicit region and size",
			body:       `{"title":"Valid","slug":"valid","instance_region":"sfo2","instance_size":"c-2"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "omitted values use defaults",
			body:       `{"title":"Defaults","slug":"defaults"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid region",
			body:       `{"title":"Invalid Region","slug":"invalid-region","instance_region":"abc","instance_size":"s-2vcpu-4gb"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  `unsupported instance region "abc"; choose one of:`,
		},
		{
			name:       "invalid size",
			body:       `{"title":"Invalid Size","slug":"invalid-size","instance_region":"nyc3","instance_size":"x"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  `unsupported instance size "x"; choose one of:`,
		},
		{
			name:       "size unavailable in region",
			body:       `{"title":"Unavailable","slug":"unavailable","instance_region":"nyc3","instance_size":"c-2"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  `instance size "c-2" is not available in region "nyc3"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, db := newTestHandler(t)
			defer db.Close()
			cookies, csrf := loginTestAdmin(t, handler)

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(tt.body))
			addCookies(request, cookies)
			request.Header.Set("X-CSRF-Token", csrf)
			handler.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if tt.wantError != "" {
				var body struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if !strings.Contains(body.Error, tt.wantError) {
					t.Fatalf("error = %q; want %q", body.Error, tt.wantError)
				}
			}
		})
	}
}

func TestNewRejectsInvalidConfiguredProvisioningDefaults(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	_, err = New(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		Auth:                newTestAuth(t),
		DefaultRegion:       "nyc3",
		DefaultInstanceSize: "c-2",
	})
	if err == nil {
		t.Fatal("New() error = nil")
	}
	if !strings.Contains(err.Error(), `instance size "c-2" is not available in region "nyc3"`) {
		t.Fatalf("New() error = %v", err)
	}
}

func TestForceDestroySessionServerRequiresAuthCSRFAndConfirmation(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()
	cookies, csrf := loginTestAdmin(t, handler)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"Destroyable","slug":"destroyable"}`))
	addCookies(createReq, cookies)
	createReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", create.Code, create.Body.String())
	}
	var created struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `update sessions set status = 'waiting_for_dns', instance_id = '123' where id = ?;`, created.Session.ID); err != nil {
		t.Fatalf("seed waiting_for_dns: %v", err)
	}

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/sessions/"+created.Session.ID+"/force-destroy", bytes.NewBufferString(`{"confirmation":"destroy destroyable"}`)))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}

	missingCSRF := httptest.NewRecorder()
	missingReq := httptest.NewRequest(http.MethodPost, "/api/sessions/"+created.Session.ID+"/force-destroy", bytes.NewBufferString(`{"confirmation":"destroy destroyable"}`))
	addCookies(missingReq, cookies)
	handler.ServeHTTP(missingCSRF, missingReq)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", missingCSRF.Code)
	}

	wrongConfirmation := httptest.NewRecorder()
	wrongReq := httptest.NewRequest(http.MethodPost, "/api/sessions/"+created.Session.ID+"/force-destroy", bytes.NewBufferString(`{"confirmation":"destroy wrong"}`))
	addCookies(wrongReq, cookies)
	wrongReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(wrongConfirmation, wrongReq)
	if wrongConfirmation.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation status = %d", wrongConfirmation.Code)
	}

	ok := httptest.NewRecorder()
	okReq := httptest.NewRequest(http.MethodPost, "/api/sessions/"+created.Session.ID+"/force-destroy", bytes.NewBufferString(`{"confirmation":"destroy destroyable"}`))
	addCookies(okReq, cookies)
	okReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(ok, okReq)
	if ok.Code != http.StatusOK {
		t.Fatalf("force destroy status = %d body = %s", ok.Code, ok.Body.String())
	}
	if !strings.Contains(ok.Body.String(), `"status":"tearing_down"`) || !strings.Contains(ok.Body.String(), "session.force_destroy_started") {
		t.Fatalf("force destroy response = %s", ok.Body.String())
	}
}

func TestJoinAPIValidatesToken(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()
	cookies, csrf := loginTestAdmin(t, handler)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"Joinable","slug":"joinable"}`))
	addCookies(createReq, cookies)
	createReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", create.Code, create.Body.String())
	}
	var created struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		Tokens map[string]struct {
			Token string `json:"token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	if _, err := db.ExecContext(context.Background(), `
update sessions
set instance_id = 'do-123', public_ip = '203.0.113.10', dns_record_id = 'cf-123', livekit_url = 'wss://livekit.example.com', recording_download_url = 'https://recordings.example.com/session.zip', finalization_summary_json = '{"files":1}', last_error = 'operator diagnostics only'
where id = ?;
`, created.Session.ID); err != nil {
		t.Fatalf("seed admin-only session fields: %v", err)
	}

	join := httptest.NewRecorder()
	handler.ServeHTTP(join, httptest.NewRequest(http.MethodGet, "/api/join/joinable?token="+created.Tokens["guest"].Token, nil))
	if join.Code != http.StatusOK {
		t.Fatalf("join status = %d body = %s", join.Code, join.Body.String())
	}
	joinBody := join.Body.String()
	if !strings.Contains(joinBody, `"role":"guest"`) || strings.Contains(joinBody, created.Tokens["guest"].Token) {
		t.Fatalf("join response invalid or leaked raw token: %s", joinBody)
	}
	for _, forbidden := range []string{"image_id", "instance_id", "public_ip", "dns_record_id", "livekit_url", "recording_download_url", "finalization_summary_json", "last_error", "access_tokens", "session_id"} {
		if strings.Contains(joinBody, forbidden) {
			t.Fatalf("join response leaked %q: %s", forbidden, joinBody)
		}
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/join/joinable?token=wrong", nil))
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("invalid status = %d", invalid.Code)
	}
}

func TestControlAppServesBuiltIndex(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile: "<div id=\"root\"></div>",
	})
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	for _, path := range []string{"/", "/sessions", "/sessions/sess_123", "/settings"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		addCookies(request, cookies)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d body = %s", path, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("GET %s Cache-Control = %q", path, response.Header().Get("Cache-Control"))
		}
		if !strings.Contains(response.Body.String(), "root") {
			t.Fatalf("GET %s body = %s", path, response.Body.String())
		}
	}
}

func TestControlPagesAndAssetsArePublicWhileSessionAPIsAreProtected(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile:       "<div id=\"control-root\"></div>",
		"assets/app.123abc.js": "console.log('control')",
		"favicon.ico":          "icon",
	})
	defer db.Close()

	for _, path := range []string{"/login", "/join/joinable?token=token", "/sessions", "/assets/app.123abc.js", "/favicon.ico"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d body = %s", path, response.Code, response.Body.String())
		}
	}

	for _, path := range []string{"/api/sessions", "/api/sessions/sess_123"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s status = %d, want %d body = %s", path, response.Code, http.StatusUnauthorized, response.Body.String())
		}
	}
}

func TestRemovedAuthAndJoinAssetPrefixesDoNotServeFiles(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile:       "<div id=\"control-root\"></div>",
		"assets/app.123abc.js": "console.log('control')",
	})
	defer db.Close()

	for _, path := range []string{"/auth-assets/assets/app.123abc.js", "/join-assets/assets/app.123abc.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d body = %s", path, response.Code, http.StatusNotFound, response.Body.String())
		}
	}
}

func TestControlAppServesAssetsWithCacheHeaders(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile:       "<div id=\"root\"></div>",
		"assets/app.123abc.js": "console.log('remote-tape')",
		"favicon.ico":          "icon",
	})
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	asset := httptest.NewRecorder()
	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.123abc.js", nil)
	addCookies(assetReq, cookies)
	handler.ServeHTTP(asset, assetReq)
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status = %d body = %s", asset.Code, asset.Body.String())
	}
	if asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset Cache-Control = %q", asset.Header().Get("Cache-Control"))
	}

	favicon := httptest.NewRecorder()
	faviconReq := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	addCookies(faviconReq, cookies)
	handler.ServeHTTP(favicon, faviconReq)
	if favicon.Code != http.StatusOK {
		t.Fatalf("favicon status = %d body = %s", favicon.Code, favicon.Body.String())
	}
	if favicon.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("favicon Cache-Control = %q", favicon.Header().Get("Cache-Control"))
	}
}

func TestControlPlaneSecurityHeaders(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile: "<div id=\"root\"></div>",
	})
	defer db.Close()

	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing Content-Security-Policy")
	}
	if response.Header().Get("Referrer-Policy") == "" {
		t.Fatal("missing Referrer-Policy")
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", response.Header().Get("X-Content-Type-Options"))
	}
	if response.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("missing Strict-Transport-Security")
	}
}

func TestControlAppDoesNotFallbackForAPIOrInvalidStaticPaths(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile: "<div id=\"root\"></div>",
	})
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	api := httptest.NewRecorder()
	apiReq := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	addCookies(apiReq, cookies)
	handler.ServeHTTP(api, apiReq)
	if api.Code != http.StatusNotFound {
		t.Fatalf("api status = %d body = %s", api.Code, api.Body.String())
	}

	asset := httptest.NewRecorder()
	assetReq := httptest.NewRequest(http.MethodGet, "/assets/%2e%2e/index.control.html", nil)
	addCookies(assetReq, cookies)
	handler.ServeHTTP(asset, assetReq)
	if asset.Code != http.StatusNotFound {
		t.Fatalf("asset status = %d body = %s", asset.Code, asset.Body.String())
	}
}

func TestControlAppReportsMissingBuild(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, nil)
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	addCookies(request, cookies)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "pnpm --dir web build:control") {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestDashboardAuthFlow(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}

	_, csrfCookie, csrf := csrfForTest(t, handler, nil)
	badLogin := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=wrong&csrf_token="+csrf))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.AddCookie(csrfCookie)
	handler.ServeHTTP(badLogin, badReq)
	if badLogin.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", badLogin.Code)
	}

	cookies, csrf := loginTestAdmin(t, handler)
	if len(cookies) == 0 {
		t.Fatal("login did not set cookies")
	}

	authenticated := httptest.NewRecorder()
	authReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	addCookies(authReq, cookies)
	handler.ServeHTTP(authenticated, authReq)
	if authenticated.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d body = %s", authenticated.Code, authenticated.Body.String())
	}

	missingCSRF := httptest.NewRecorder()
	missingReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"No CSRF","slug":"no-csrf"}`))
	addCookies(missingReq, cookies)
	handler.ServeHTTP(missingCSRF, missingReq)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", missingCSRF.Code)
	}

	created := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"With CSRF","slug":"with-csrf"}`))
	addCookies(createReq, cookies)
	createReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(created, createReq)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", created.Code, created.Body.String())
	}

	logout := httptest.NewRecorder()
	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	addCookies(logoutReq, cookies)
	logoutReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(logout, logoutReq)
	if logout.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d", logout.Code)
	}
	cleared := responseCookies(logout)["remote_tape_session"]
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatalf("logout did not clear session cookie: %#v", cleared)
	}

	postLogout := httptest.NewRecorder()
	postLogoutReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	for name, cookie := range cookies {
		if name != "remote_tape_session" {
			postLogoutReq.AddCookie(cookie)
		}
	}
	handler.ServeHTTP(postLogout, postLogoutReq)
	if postLogout.Code != http.StatusUnauthorized {
		t.Fatalf("post logout status = %d", postLogout.Code)
	}
}

func ptr(value string) *string {
	return &value
}

func equalPtr(a *string, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func newTestHandler(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := database.Migrate(ctx, db, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		db.Close()
		t.Fatalf("Migrate() error = %v", err)
	}
	handler, err := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Auth: newTestAuth(t)})
	if err != nil {
		db.Close()
		t.Fatalf("New() error = %v", err)
	}
	return handler, db
}

func newTestHandlerWithControlDist(t *testing.T, files map[string]string) (http.Handler, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := database.Migrate(ctx, db, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		db.Close()
		t.Fatalf("Migrate() error = %v", err)
	}
	handler, err := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Auth: newTestAuth(t), controlUIFS: testFS(files)})
	if err != nil {
		db.Close()
		t.Fatalf("New() error = %v", err)
	}
	return handler, db
}

func testFS(files map[string]string) fs.FS {
	fileSystem := fstest.MapFS{}
	for name, content := range files {
		fileSystem[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fileSystem
}

func newTestAuth(t *testing.T) *auth.Manager {
	t.Helper()
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	manager, err := auth.NewManager(auth.Config{
		PasswordHash:         hash,
		CookieAuthKey:        []byte("0123456789abcdef0123456789abcdef"),
		CookieEncryptionKey:  []byte("0123456789abcdef0123456789abcdef"),
		SessionDuration:      time.Hour,
		RateLimitMaxAttempts: 5,
		RateLimitWindow:      15 * time.Minute,
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func loginTestAdmin(t *testing.T, handler http.Handler) (map[string]*http.Cookie, string) {
	t.Helper()
	_, csrfCookie, csrf := csrfForTest(t, handler, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=correct-password&csrf_token="+csrf))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(csrfCookie)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d body = %s", response.Code, response.Body.String())
	}
	cookies := responseCookies(response)
	if cookies["remote_tape_session"] == nil {
		t.Fatalf("login missing session cookie: %#v", cookies)
	}
	_, csrfCookie, csrf = csrfForTest(t, handler, cookies)
	cookies[csrfCookie.Name] = csrfCookie
	return cookies, csrf
}

func csrfForTest(t *testing.T, handler http.Handler, cookies map[string]*http.Cookie) (map[string]*http.Cookie, *http.Cookie, string) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	addCookies(request, cookies)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("auth session status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode auth session: %v", err)
	}
	csrfCookie := responseCookies(response)["remote_tape_csrf"]
	if csrfCookie == nil || body.CSRFToken == "" {
		t.Fatalf("missing csrf cookie/token: cookie=%#v body=%s", csrfCookie, response.Body.String())
	}
	return responseCookies(response), csrfCookie, body.CSRFToken
}

func addCookies(request *http.Request, cookies map[string]*http.Cookie) {
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
}

func responseCookies(response *httptest.ResponseRecorder) map[string]*http.Cookie {
	cookies := map[string]*http.Cookie{}
	for _, cookie := range response.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	return cookies
}
