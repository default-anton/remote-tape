package dns

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflareZoneDiscoveryUsesLongestSuffixAndCaches(t *testing.T) {
	ctx := context.Background()
	fake := newFakeCloudflare(t)
	fake.zones["example.com"] = "zone_example"
	manager := fake.manager("sessions.example.com")

	if err := manager.Validate(ctx); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := manager.Validate(ctx); err != nil {
		t.Fatalf("second Validate() error = %v", err)
	}
	if got := strings.Join(fake.zoneQueries, ","); got != "sessions.example.com,example.com" {
		t.Fatalf("zone queries = %q", got)
	}
}

func TestCloudflareEnsureCreatesMissingARecord(t *testing.T) {
	ctx := context.Background()
	fake := newFakeCloudflare(t)
	manager := fake.manager("sessions.example.com")

	result, err := manager.EnsureARecord(ctx, ensureInput(""))
	if err != nil {
		t.Fatalf("EnsureARecord() error = %v", err)
	}
	if result.Operation != "created" || result.ID == "" || result.ZoneID != "zone_example" {
		t.Fatalf("result = %+v", result)
	}
	record := fake.records[result.ID]
	if record.Type != "A" || record.Name != "room-opaque.sessions.example.com" || record.Content != "203.0.113.10" || record.TTL != RequiredTTL || record.Proxied {
		t.Fatalf("created record = %+v", record)
	}
}

func TestCloudflareEnsureAdoptsExistingCorrectRecord(t *testing.T) {
	ctx := context.Background()
	fake := newFakeCloudflare(t)
	fake.records["dns_existing"] = cloudflareRecord{ID: "dns_existing", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL, Proxied: false}
	manager := fake.manager("sessions.example.com")

	result, err := manager.EnsureARecord(ctx, ensureInput(""))
	if err != nil {
		t.Fatalf("EnsureARecord() error = %v", err)
	}
	if result.ID != "dns_existing" || result.Operation != "adopted" || fake.updates != 0 {
		t.Fatalf("result = %+v updates=%d", result, fake.updates)
	}
}

func TestCloudflareEnsureUpdatesWrongIPTTLOrProxiedRecord(t *testing.T) {
	for _, record := range []cloudflareRecord{
		{ID: "dns_wrong_ip", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.11", TTL: RequiredTTL, Proxied: false},
		{ID: "dns_wrong_ttl", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: 300, Proxied: false},
		{ID: "dns_proxied", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL, Proxied: true},
	} {
		t.Run(record.ID, func(t *testing.T) {
			ctx := context.Background()
			fake := newFakeCloudflare(t)
			fake.records[record.ID] = record
			manager := fake.manager("sessions.example.com")

			result, err := manager.EnsureARecord(ctx, ensureInput(""))
			if err != nil {
				t.Fatalf("EnsureARecord() error = %v", err)
			}
			updated := fake.records[record.ID]
			if result.Operation != "updated" || updated.Content != "203.0.113.10" || updated.TTL != RequiredTTL || updated.Proxied {
				t.Fatalf("result = %+v updated=%+v", result, updated)
			}
		})
	}
}

func TestCloudflareEnsureErrorsOnAmbiguousExactNameRecords(t *testing.T) {
	ctx := context.Background()
	fake := newFakeCloudflare(t)
	fake.records["dns_one"] = cloudflareRecord{ID: "dns_one", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL}
	fake.records["dns_two"] = cloudflareRecord{ID: "dns_two", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL}
	manager := fake.manager("sessions.example.com")

	_, err := manager.EnsureARecord(ctx, ensureInput(""))
	if err == nil || !strings.Contains(err.Error(), "multiple records") {
		t.Fatalf("EnsureARecord() error = %v", err)
	}
}

func TestCloudflareEnsureErrorsWhenStoredIDHasExactNameConflicts(t *testing.T) {
	ctx := context.Background()
	fake := newFakeCloudflare(t)
	fake.records["dns_stored"] = cloudflareRecord{ID: "dns_stored", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL, Proxied: false}
	fake.records["dns_duplicate"] = cloudflareRecord{ID: "dns_duplicate", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL, Proxied: false}
	manager := fake.manager("sessions.example.com")

	_, err := manager.EnsureARecord(ctx, ensureInput("dns_stored"))
	if err == nil || !strings.Contains(err.Error(), "not the only record") {
		t.Fatalf("EnsureARecord() error = %v", err)
	}
}

func TestCloudflareEnsureErrorsOnSameNameNonAConflict(t *testing.T) {
	ctx := context.Background()
	fake := newFakeCloudflare(t)
	fake.records["dns_cname"] = cloudflareRecord{ID: "dns_cname", Type: "CNAME", Name: "room-opaque.sessions.example.com", Content: "other.example.com", TTL: RequiredTTL}
	manager := fake.manager("sessions.example.com")

	_, err := manager.EnsureARecord(ctx, ensureInput(""))
	if err == nil || !strings.Contains(err.Error(), "non-A") {
		t.Fatalf("EnsureARecord() error = %v", err)
	}
}

func TestCloudflareEnsureRepairsStalePersistedIDViaLookup(t *testing.T) {
	ctx := context.Background()
	fake := newFakeCloudflare(t)
	fake.records["dns_stale"] = cloudflareRecord{ID: "dns_stale", Type: "A", Name: "other.sessions.example.com", Content: "203.0.113.55", TTL: RequiredTTL}
	fake.records["dns_room"] = cloudflareRecord{ID: "dns_room", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.99", TTL: 300, Proxied: true}
	manager := fake.manager("sessions.example.com")

	result, err := manager.EnsureARecord(ctx, ensureInput("dns_stale"))
	if err != nil {
		t.Fatalf("EnsureARecord() error = %v", err)
	}
	if result.ID != "dns_room" || result.Operation != "updated" {
		t.Fatalf("result = %+v", result)
	}
}

func TestCloudflareDeleteVerifiesIDTreatsMissingAsSuccessAndRefusesAmbiguousLookup(t *testing.T) {
	t.Run("delete by id", func(t *testing.T) {
		ctx := context.Background()
		fake := newFakeCloudflare(t)
		fake.records["dns_delete"] = cloudflareRecord{ID: "dns_delete", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL}
		manager := fake.manager("sessions.example.com")

		if err := manager.DeleteRecord(ctx, DeleteRecordInput{SessionID: "sess_test", RoomDomain: "room-opaque.sessions.example.com", DNSRecordID: "dns_delete", BaseDomain: "sessions.example.com"}); err != nil {
			t.Fatalf("DeleteRecord() error = %v", err)
		}
		if _, ok := fake.records["dns_delete"]; ok {
			t.Fatal("record was not deleted")
		}
	})

	t.Run("stale id conflict fails", func(t *testing.T) {
		ctx := context.Background()
		fake := newFakeCloudflare(t)
		fake.records["dns_other"] = cloudflareRecord{ID: "dns_other", Type: "A", Name: "other.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL}
		manager := fake.manager("sessions.example.com")

		err := manager.DeleteRecord(ctx, DeleteRecordInput{SessionID: "sess_test", RoomDomain: "room-opaque.sessions.example.com", DNSRecordID: "dns_other", BaseDomain: "sessions.example.com"})
		if err == nil || !strings.Contains(err.Error(), "belongs to") {
			t.Fatalf("DeleteRecord() error = %v", err)
		}
		if _, ok := fake.records["dns_other"]; !ok {
			t.Fatal("stale record was deleted")
		}
	})

	t.Run("missing is success", func(t *testing.T) {
		ctx := context.Background()
		fake := newFakeCloudflare(t)
		manager := fake.manager("sessions.example.com")
		if err := manager.DeleteRecord(ctx, DeleteRecordInput{SessionID: "sess_test", RoomDomain: "room-opaque.sessions.example.com", DNSRecordID: "dns_missing", BaseDomain: "sessions.example.com"}); err != nil {
			t.Fatalf("DeleteRecord() error = %v", err)
		}
	})

	t.Run("ambiguous lookup fails", func(t *testing.T) {
		ctx := context.Background()
		fake := newFakeCloudflare(t)
		fake.records["dns_one"] = cloudflareRecord{ID: "dns_one", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL}
		fake.records["dns_two"] = cloudflareRecord{ID: "dns_two", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL}
		manager := fake.manager("sessions.example.com")
		if err := manager.DeleteRecord(ctx, DeleteRecordInput{SessionID: "sess_test", RoomDomain: "room-opaque.sessions.example.com", BaseDomain: "sessions.example.com"}); err == nil || !strings.Contains(err.Error(), "multiple") {
			t.Fatalf("DeleteRecord() error = %v", err)
		}
	})

	t.Run("proxied lookup fails", func(t *testing.T) {
		ctx := context.Background()
		fake := newFakeCloudflare(t)
		fake.records["dns_proxied"] = cloudflareRecord{ID: "dns_proxied", Type: "A", Name: "room-opaque.sessions.example.com", Content: "203.0.113.10", TTL: RequiredTTL, Proxied: true}
		manager := fake.manager("sessions.example.com")
		if err := manager.DeleteRecord(ctx, DeleteRecordInput{SessionID: "sess_test", RoomDomain: "room-opaque.sessions.example.com", BaseDomain: "sessions.example.com"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("DeleteRecord() error = %v", err)
		}
	})
}

func TestCloudflareDoesNotReturnTokenInErrors(t *testing.T) {
	ctx := context.Background()
	fake := newFakeCloudflare(t)
	fake.failHTTP = true
	manager := fake.manager("sessions.example.com")

	_, err := manager.EnsureARecord(ctx, ensureInput(""))
	if err == nil {
		t.Fatal("EnsureARecord() error = nil")
	}
	if strings.Contains(err.Error(), fake.token) {
		t.Fatalf("error leaked token: %v", err)
	}
}

type fakeCloudflare struct {
	t      *testing.T
	server *httptest.Server
	token  string

	zones       map[string]string
	records     map[string]cloudflareRecord
	zoneQueries []string
	updates     int
	failHTTP    bool
}

func newFakeCloudflare(t *testing.T) *fakeCloudflare {
	fake := &fakeCloudflare{
		t:       t,
		token:   "cf_test_secret",
		zones:   map[string]string{"example.com": "zone_example"},
		records: map[string]cloudflareRecord{},
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeCloudflare) manager(baseDomain string) *CloudflareManager {
	manager, err := NewCloudflareManager(CloudflareConfig{APIToken: f.token, BaseDomain: baseDomain, APIBaseURL: f.server.URL, HTTPClient: f.server.Client()})
	if err != nil {
		f.t.Fatalf("NewCloudflareManager() error = %v", err)
	}
	return manager
}

func (f *fakeCloudflare) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		f.t.Fatalf("Authorization header = %q", r.Header.Get("Authorization"))
	}
	if f.failHTTP {
		http.Error(w, "cloudflare exploded", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet && r.URL.Path == "/client/v4/zones" {
		name := r.URL.Query().Get("name")
		f.zoneQueries = append(f.zoneQueries, name)
		var result []cloudflareZone
		if id := f.zones[name]; id != "" {
			result = append(result, cloudflareZone{ID: id, Name: name})
		}
		writeCFList(w, result)
		return
	}

	prefix := "/client/v4/zones/zone_example/dns_records"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	switch r.Method {
	case http.MethodGet:
		if id != "" {
			record, ok := f.records[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeCFObject(w, record)
			return
		}
		name := r.URL.Query().Get("name")
		var result []cloudflareRecord
		for _, record := range f.records {
			if record.Name == name {
				result = append(result, record)
			}
		}
		writeCFList(w, result)
	case http.MethodPost:
		var req recordRequest
		decodeJSON(f.t, r, &req)
		if req.Type != "A" || req.TTL != RequiredTTL || req.Proxied || req.Name == "" || req.Content == "" {
			f.t.Fatalf("create request = %+v", req)
		}
		record := cloudflareRecord{ID: "dns_created", Type: req.Type, Name: req.Name, Content: req.Content, TTL: req.TTL, Proxied: req.Proxied}
		f.records[record.ID] = record
		writeCFObject(w, record)
	case http.MethodPut:
		var req recordRequest
		decodeJSON(f.t, r, &req)
		record := cloudflareRecord{ID: id, Type: req.Type, Name: req.Name, Content: req.Content, TTL: req.TTL, Proxied: req.Proxied}
		f.records[id] = record
		f.updates++
		writeCFObject(w, record)
	case http.MethodDelete:
		delete(f.records, id)
		writeCFDelete(w)
	default:
		http.NotFound(w, r)
	}
}

func ensureInput(recordID string) EnsureARecordInput {
	return EnsureARecordInput{SessionID: "sess_test", RoomDomain: "room-opaque.sessions.example.com", PublicIP: "203.0.113.10", DNSRecordID: recordID, BaseDomain: "sessions.example.com"}
}

func decodeJSON(t *testing.T, r *http.Request, out any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}

func writeCFList[T any](w http.ResponseWriter, result []T) {
	_ = json.NewEncoder(w).Encode(cloudflareListResponse[T]{Success: true, Result: result, ResultInfo: cloudflareResultInfo{Page: 1, TotalPages: 1}})
}

func writeCFObject[T any](w http.ResponseWriter, result T) {
	_ = json.NewEncoder(w).Encode(cloudflareObjectResponse[T]{Success: true, Result: result})
}

func writeCFDelete(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(cloudflareDeleteResponse{Success: true})
}
