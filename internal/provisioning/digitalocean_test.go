package provisioning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/default-anton/remote-tape/internal/session"
	"github.com/digitalocean/godo"
)

func TestDigitalOceanEnsureInstanceCreatesWhenMissing(t *testing.T) {
	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/tags":
			writeDOJSON(w, http.StatusCreated, map[string]any{"tag": map[string]any{"name": "ok"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets":
			writeDOJSON(w, http.StatusOK, map[string]any{"droplets": []any{}, "links": map[string]any{}, "meta": map[string]any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/droplets":
			created = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			if !containsString(body["tags"].([]any), baseTag) || !containsString(body["tags"].([]any), "remote-tape-session:sess_1") {
				t.Fatalf("create tags = %#v", body["tags"])
			}
			if got := len(body["ssh_keys"].([]any)); got != 1 {
				t.Fatalf("ssh_keys len = %d", got)
			}
			writeDOJSON(w, http.StatusAccepted, dropletPayload(42, []string{baseTag, "remote-tape-session:sess_1"}, "203.0.113.42"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	result, err := testDOProvider(server).EnsureInstance(context.Background(), testSession())
	if err != nil {
		t.Fatalf("EnsureInstance() error = %v", err)
	}
	if !created || result.ID != "42" || result.IP != "203.0.113.42" || result.Adopted {
		t.Fatalf("result = %#v created=%v", result, created)
	}
}

func TestDigitalOceanEnsureInstanceAdoptsAndRepairsTags(t *testing.T) {
	var repaired bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/tags":
			writeDOJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "tag already exists"})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets":
			writeDOJSON(w, http.StatusOK, map[string]any{"droplets": []any{droplet(99, []string{"remote-tape-session:sess_1"}, "203.0.113.99")}, "links": map[string]any{}, "meta": map[string]any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resources"):
			repaired = true
			writeDOJSON(w, http.StatusNoContent, map[string]any{})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	result, err := testDOProvider(server).EnsureInstance(context.Background(), testSession())
	if err != nil {
		t.Fatalf("EnsureInstance() error = %v", err)
	}
	if !result.Adopted || result.ID != "99" || result.IP != "203.0.113.99" || !repaired {
		t.Fatalf("result = %#v repaired=%v", result, repaired)
	}
}

func TestDigitalOceanEnsureInstancePrefersPersistedInstanceID(t *testing.T) {
	var listed, created, repaired bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/tags":
			writeDOJSON(w, http.StatusCreated, map[string]any{"tag": map[string]any{"name": "ok"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets/123":
			writeDOJSON(w, http.StatusOK, dropletPayload(123, []string{}, "203.0.113.123"))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resources"):
			repaired = true
			writeDOJSON(w, http.StatusNoContent, map[string]any{})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets":
			listed = true
			writeDOJSON(w, http.StatusOK, map[string]any{"droplets": []any{}, "links": map[string]any{}, "meta": map[string]any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/droplets":
			created = true
			writeDOJSON(w, http.StatusAccepted, dropletPayload(456, []string{baseTag, "remote-tape-session:sess_1"}, "203.0.113.45"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	s := testSession()
	dropletID := "123"
	s.InstanceID = &dropletID
	result, err := testDOProvider(server).EnsureInstance(context.Background(), s)
	if err != nil {
		t.Fatalf("EnsureInstance() error = %v", err)
	}
	if !result.Adopted || result.ID != "123" || result.IP != "203.0.113.123" || !repaired || listed || created {
		t.Fatalf("result = %#v repaired=%v listed=%v created=%v", result, repaired, listed, created)
	}
}

func TestDigitalOceanEnsureInstanceFallsBackToTagWhenPersistedDropletIsMissing(t *testing.T) {
	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/tags":
			writeDOJSON(w, http.StatusCreated, map[string]any{"tag": map[string]any{"name": "ok"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets/123":
			writeDOJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets":
			writeDOJSON(w, http.StatusOK, map[string]any{"droplets": []any{droplet(456, []string{baseTag, "remote-tape-session:sess_1"}, "203.0.113.45")}, "links": map[string]any{}, "meta": map[string]any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/droplets":
			created = true
			writeDOJSON(w, http.StatusAccepted, dropletPayload(789, []string{baseTag, "remote-tape-session:sess_1"}, "203.0.113.78"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	s := testSession()
	dropletID := "123"
	s.InstanceID = &dropletID
	result, err := testDOProvider(server).EnsureInstance(context.Background(), s)
	if err != nil {
		t.Fatalf("EnsureInstance() error = %v", err)
	}
	if !result.Adopted || result.ID != "456" || result.IP != "203.0.113.45" || created {
		t.Fatalf("result = %#v created=%v", result, created)
	}
}

func TestDigitalOceanForceDestroyFallsBackToTaggedDropletsWhenPersistedIDIsMissing(t *testing.T) {
	deleted := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/droplets/123":
			writeDOJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets":
			writeDOJSON(w, http.StatusOK, map[string]any{"droplets": []any{droplet(456, []string{"remote-tape-session:sess_1"}, "203.0.113.45")}, "links": map[string]any{}, "meta": map[string]any{}})
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/droplets/456":
			deleted["456"] = true
			writeDOJSON(w, http.StatusNoContent, map[string]any{})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	dropletID := "123"
	result, err := testDOProvider(server).ForceDestroySessionServer(context.Background(), session.Session{ID: "sess_1", InstanceID: &dropletID})
	if err != nil {
		t.Fatalf("ForceDestroySessionServer() error = %v", err)
	}
	if result.InstanceID != "456" || !deleted["456"] {
		t.Fatalf("result = %#v deleted=%v", result, deleted)
	}
}

func TestDigitalOceanEnsureInstanceReturnsNoIPForRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/tags":
			writeDOJSON(w, http.StatusCreated, map[string]any{"tag": map[string]any{"name": "ok"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/droplets":
			writeDOJSON(w, http.StatusOK, map[string]any{"droplets": []any{}, "links": map[string]any{}, "meta": map[string]any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/droplets":
			writeDOJSON(w, http.StatusAccepted, dropletPayload(77, []string{baseTag, "remote-tape-session:sess_1"}, ""))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	result, err := testDOProvider(server).EnsureInstance(context.Background(), testSession())
	if err != nil {
		t.Fatalf("EnsureInstance() error = %v", err)
	}
	if result.ID != "77" || result.IP != "" {
		t.Fatalf("result = %#v", result)
	}
}

func testDOProvider(server *httptest.Server) *DigitalOceanInstanceProvider {
	client, err := godo.New(nil, godo.SetBaseURL(server.URL+"/"))
	if err != nil {
		panic(err)
	}
	return &DigitalOceanInstanceProvider{
		client:  client,
		sshKeys: []godo.DropletCreateSSHKey{{ID: 12345}},
	}
}

func testSession() session.Session {
	return session.Session{ID: "sess_1", Slug: "demo", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "ubuntu-24-04-x64"}
}

func dropletPayload(id int, tags []string, ip string) map[string]any {
	return map[string]any{"droplet": droplet(id, tags, ip)}
}

func droplet(id int, tags []string, ip string) map[string]any {
	v4 := []any{}
	if ip != "" {
		v4 = append(v4, map[string]any{"ip_address": ip, "type": "public"})
	}
	return map[string]any{"id": id, "name": "remote-tape-demo", "tags": tags, "networks": map[string]any{"v4": v4}}
}

func writeDOJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func containsString(values []any, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
