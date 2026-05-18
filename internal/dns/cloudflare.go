package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

const defaultCloudflareAPIBase = "https://api.cloudflare.com"

type CloudflareConfig struct {
	APIToken   string
	BaseDomain string
	APIBaseURL string
	HTTPClient *http.Client
}

type CloudflareManager struct {
	apiToken   string
	baseDomain string
	apiBaseURL *url.URL
	client     *http.Client

	mu       sync.Mutex
	zoneID   string
	zoneName string
}

type cloudflareListResponse[T any] struct {
	Success    bool                   `json:"success"`
	Errors     []cloudflareAPIMessage `json:"errors"`
	Result     []T                    `json:"result"`
	ResultInfo cloudflareResultInfo   `json:"result_info"`
}

type cloudflareObjectResponse[T any] struct {
	Success bool                   `json:"success"`
	Errors  []cloudflareAPIMessage `json:"errors"`
	Result  T                      `json:"result"`
}

type cloudflareDeleteResponse struct {
	Success bool                   `json:"success"`
	Errors  []cloudflareAPIMessage `json:"errors"`
}

type cloudflareAPIMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cloudflareResultInfo struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

type cloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cloudflareRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type recordRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

func NewCloudflareManager(cfg CloudflareConfig) (*CloudflareManager, error) {
	token := strings.TrimSpace(cfg.APIToken)
	if token == "" {
		return nil, fmt.Errorf("%w: cloudflare api token is required", ErrConfiguration)
	}
	baseDomain, err := normalizeDNSName(cfg.BaseDomain)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid sessions base domain: %v", ErrConfiguration, err)
	}
	apiBase := strings.TrimSpace(cfg.APIBaseURL)
	if apiBase == "" {
		apiBase = defaultCloudflareAPIBase
	}
	parsed, err := url.Parse(apiBase)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: cloudflare api base url must be absolute", ErrConfiguration)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &CloudflareManager{apiToken: token, baseDomain: baseDomain, apiBaseURL: parsed, client: client}, nil
}

func (m *CloudflareManager) Validate(ctx context.Context) error {
	_, _, err := m.zone(ctx)
	return err
}

func (m *CloudflareManager) EnsureARecord(ctx context.Context, input EnsureARecordInput) (RecordResult, error) {
	roomDomain, publicIP, baseDomain, err := normalizeEnsureInput(input)
	if err != nil {
		return RecordResult{}, err
	}
	zoneID, _, err := m.zoneForBaseDomain(ctx, baseDomain)
	if err != nil {
		return RecordResult{}, err
	}
	result, err := m.ensureARecordInZone(ctx, zoneID, roomDomain, publicIP, input.DNSRecordID)
	if err != nil {
		return RecordResult{}, OperationError{ZoneID: zoneID, Err: err}
	}
	return result, nil
}

func (m *CloudflareManager) ensureARecordInZone(ctx context.Context, zoneID string, roomDomain string, publicIP string, dnsRecordID string) (RecordResult, error) {
	storedRecordID := strings.TrimSpace(dnsRecordID)
	if storedRecordID != "" {
		record, found, err := m.getRecord(ctx, zoneID, storedRecordID)
		if err != nil {
			return RecordResult{}, err
		}
		if found && exactARecord(record, roomDomain) {
			return m.ensureSingleExactRecord(ctx, zoneID, roomDomain, publicIP, record.ID)
		}
	}

	records, err := m.listRecordsByName(ctx, zoneID, roomDomain)
	if err != nil {
		return RecordResult{}, err
	}
	return m.ensureFromExactRecords(ctx, zoneID, roomDomain, publicIP, records)
}

func (m *CloudflareManager) DeleteRecord(ctx context.Context, input DeleteRecordInput) error {
	roomDomain, baseDomain, err := normalizeDeleteInput(input)
	if err != nil {
		return err
	}
	zoneID, _, err := m.zoneForBaseDomain(ctx, baseDomain)
	if err != nil {
		return err
	}
	if err := m.deleteRecordInZone(ctx, zoneID, roomDomain, input.DNSRecordID); err != nil {
		return OperationError{ZoneID: zoneID, Err: err}
	}
	return nil
}

func (m *CloudflareManager) deleteRecordInZone(ctx context.Context, zoneID string, roomDomain string, dnsRecordID string) error {
	if strings.TrimSpace(dnsRecordID) != "" {
		record, found, err := m.getRecord(ctx, zoneID, strings.TrimSpace(dnsRecordID))
		if err != nil {
			return err
		}
		if found {
			if !sameName(record.Name, roomDomain) || record.Type != "A" {
				return fmt.Errorf("%w: stored DNS record id %s belongs to %s/%s, not A %s", ErrConflict, dnsRecordID, record.Type, record.Name, roomDomain)
			}
			return m.deleteRecordByID(ctx, zoneID, record.ID)
		}
	}

	records, err := m.listRecordsByName(ctx, zoneID, roomDomain)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	if len(records) > 1 {
		return fmt.Errorf("%w: multiple records exist for %s; delete manually before retrying", ErrConflict, roomDomain)
	}
	record := records[0]
	if !sameName(record.Name, roomDomain) || record.Type != "A" || record.Proxied {
		return fmt.Errorf("%w: ambiguous DNS record exists for %s; delete manually before retrying", ErrConflict, roomDomain)
	}
	return m.deleteRecordByID(ctx, zoneID, record.ID)
}

func (m *CloudflareManager) zone(ctx context.Context) (string, string, error) {
	return m.zoneForBaseDomain(ctx, m.baseDomain)
}

func (m *CloudflareManager) zoneForBaseDomain(ctx context.Context, baseDomain string) (string, string, error) {
	baseDomain, err := normalizeDNSName(baseDomain)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid sessions base domain: %v", ErrConfiguration, err)
	}

	m.mu.Lock()
	if m.zoneID != "" && dnsNameWithin(baseDomain, m.zoneName) {
		zoneID, zoneName := m.zoneID, m.zoneName
		m.mu.Unlock()
		return zoneID, zoneName, nil
	}
	m.mu.Unlock()

	for _, candidate := range zoneCandidates(baseDomain) {
		zones, err := m.listZones(ctx, candidate)
		if err != nil {
			return "", "", err
		}
		if len(zones) == 0 {
			continue
		}
		if len(zones) > 1 {
			return "", "", fmt.Errorf("%w: multiple Cloudflare zones match %s", ErrConfiguration, candidate)
		}
		zone := zones[0]
		if strings.TrimSpace(zone.ID) == "" {
			return "", "", fmt.Errorf("%w: Cloudflare zone %s has empty id", ErrConfiguration, zone.Name)
		}
		m.mu.Lock()
		m.zoneID, m.zoneName = zone.ID, strings.ToLower(strings.Trim(zone.Name, "."))
		m.mu.Unlock()
		return zone.ID, zone.Name, nil
	}
	return "", "", fmt.Errorf("%w: no Cloudflare zone found for %s; token needs Zone:Zone:Read on owning zone", ErrConfiguration, baseDomain)
}

func (m *CloudflareManager) listZones(ctx context.Context, name string) ([]cloudflareZone, error) {
	var response cloudflareListResponse[cloudflareZone]
	if err := m.do(ctx, http.MethodGet, "/client/v4/zones", url.Values{"name": {name}}, nil, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, cloudflareError("list zones", response.Errors)
	}
	return response.Result, nil
}

func (m *CloudflareManager) getRecord(ctx context.Context, zoneID string, id string) (cloudflareRecord, bool, error) {
	var response cloudflareObjectResponse[cloudflareRecord]
	err := m.do(ctx, http.MethodGet, "/client/v4/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(id), nil, nil, &response)
	if err != nil {
		if isNotFoundError(err) {
			return cloudflareRecord{}, false, nil
		}
		return cloudflareRecord{}, false, err
	}
	if !response.Success {
		if cloudflareMessagesNotFound(response.Errors) {
			return cloudflareRecord{}, false, nil
		}
		return cloudflareRecord{}, false, cloudflareError("get dns record", response.Errors)
	}
	return response.Result, true, nil
}

func (m *CloudflareManager) listRecordsByName(ctx context.Context, zoneID string, name string) ([]cloudflareRecord, error) {
	var all []cloudflareRecord
	page := 1
	for {
		var response cloudflareListResponse[cloudflareRecord]
		query := url.Values{"name": {name}, "per_page": {"100"}, "page": {fmt.Sprint(page)}}
		if err := m.do(ctx, http.MethodGet, "/client/v4/zones/"+url.PathEscape(zoneID)+"/dns_records", query, nil, &response); err != nil {
			return nil, err
		}
		if !response.Success {
			return nil, cloudflareError("list dns records", response.Errors)
		}
		all = append(all, response.Result...)
		if response.ResultInfo.TotalPages <= 1 || page >= response.ResultInfo.TotalPages {
			break
		}
		page++
	}
	return all, nil
}

func (m *CloudflareManager) createRecord(ctx context.Context, zoneID string, name string, publicIP string) (cloudflareRecord, error) {
	var response cloudflareObjectResponse[cloudflareRecord]
	payload := recordRequest{Type: "A", Name: name, Content: publicIP, TTL: RequiredTTL, Proxied: false}
	if err := m.do(ctx, http.MethodPost, "/client/v4/zones/"+url.PathEscape(zoneID)+"/dns_records", nil, payload, &response); err != nil {
		return cloudflareRecord{}, err
	}
	if !response.Success {
		return cloudflareRecord{}, cloudflareError("create dns record", response.Errors)
	}
	return response.Result, nil
}

func (m *CloudflareManager) updateRecord(ctx context.Context, zoneID string, id string, name string, publicIP string) (cloudflareRecord, error) {
	var response cloudflareObjectResponse[cloudflareRecord]
	payload := recordRequest{Type: "A", Name: name, Content: publicIP, TTL: RequiredTTL, Proxied: false}
	if err := m.do(ctx, http.MethodPut, "/client/v4/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(id), nil, payload, &response); err != nil {
		return cloudflareRecord{}, err
	}
	if !response.Success {
		return cloudflareRecord{}, cloudflareError("update dns record", response.Errors)
	}
	return response.Result, nil
}

func (m *CloudflareManager) deleteRecordByID(ctx context.Context, zoneID string, id string) error {
	var response cloudflareDeleteResponse
	err := m.do(ctx, http.MethodDelete, "/client/v4/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(id), nil, nil, &response)
	if err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return err
	}
	if !response.Success {
		if cloudflareMessagesNotFound(response.Errors) {
			return nil
		}
		return cloudflareError("delete dns record", response.Errors)
	}
	return nil
}

func (m *CloudflareManager) ensureSingleExactRecord(ctx context.Context, zoneID string, roomDomain string, publicIP string, expectedRecordID string) (RecordResult, error) {
	records, err := m.listRecordsByName(ctx, zoneID, roomDomain)
	if err != nil {
		return RecordResult{}, err
	}
	if len(records) != 1 {
		return RecordResult{}, fmt.Errorf("%w: stored DNS record id %s is not the only record for %s; remove duplicates before retrying", ErrConflict, expectedRecordID, roomDomain)
	}
	if records[0].ID != expectedRecordID {
		return RecordResult{}, fmt.Errorf("%w: stored DNS record id %s does not match the exact-name record for %s", ErrConflict, expectedRecordID, roomDomain)
	}
	return m.ensureFromExactRecords(ctx, zoneID, roomDomain, publicIP, records)
}

func (m *CloudflareManager) ensureFromExactRecords(ctx context.Context, zoneID string, roomDomain string, publicIP string, records []cloudflareRecord) (RecordResult, error) {
	if len(records) == 0 {
		created, err := m.createRecord(ctx, zoneID, roomDomain, publicIP)
		if err != nil {
			return RecordResult{}, err
		}
		return RecordResult{ID: created.ID, ZoneID: zoneID, Name: created.Name, Content: created.Content, Operation: "created"}, nil
	}
	if len(records) > 1 {
		return RecordResult{}, fmt.Errorf("%w: multiple records exist for %s; remove duplicates before retrying", ErrConflict, roomDomain)
	}
	record := records[0]
	if !sameName(record.Name, roomDomain) || record.Type != "A" {
		return RecordResult{}, fmt.Errorf("%w: non-A record exists for %s; remove it before retrying", ErrConflict, roomDomain)
	}
	return m.repairOrAdopt(ctx, zoneID, record, publicIP, "adopted")
}

func (m *CloudflareManager) repairOrAdopt(ctx context.Context, zoneID string, record cloudflareRecord, publicIP string, adoptOperation string) (RecordResult, error) {
	if record.Content == publicIP && record.TTL == RequiredTTL && !record.Proxied {
		return RecordResult{ID: record.ID, ZoneID: zoneID, Name: record.Name, Content: record.Content, Operation: adoptOperation}, nil
	}
	updated, err := m.updateRecord(ctx, zoneID, record.ID, record.Name, publicIP)
	if err != nil {
		return RecordResult{}, err
	}
	return RecordResult{ID: updated.ID, ZoneID: zoneID, Name: updated.Name, Content: updated.Content, Operation: "updated"}, nil
}

func (m *CloudflareManager) do(ctx context.Context, method string, rawPath string, query url.Values, body any, out any) error {
	endpoint := *m.apiBaseURL
	endpoint.Path = path.Join(endpoint.Path, rawPath)
	endpoint.RawQuery = query.Encode()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal cloudflare request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return fmt.Errorf("build cloudflare request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare %s %s: %w", method, rawPath, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read cloudflare response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return notFoundError{op: method + " " + rawPath}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare %s %s returned HTTP %d: %s", method, rawPath, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode cloudflare response: %w", err)
	}
	return nil
}

func normalizeEnsureInput(input EnsureARecordInput) (string, string, string, error) {
	roomDomain, err := normalizeDNSName(input.RoomDomain)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: invalid room domain: %v", ErrConfiguration, err)
	}
	baseDomain, err := normalizeDNSName(input.BaseDomain)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: invalid sessions base domain: %v", ErrConfiguration, err)
	}
	if !dnsNameWithin(roomDomain, baseDomain) || roomDomain == baseDomain {
		return "", "", "", fmt.Errorf("%w: room domain %s is not under sessions base domain %s", ErrConfiguration, roomDomain, baseDomain)
	}
	publicIP := strings.TrimSpace(input.PublicIP)
	parsed := net.ParseIP(publicIP)
	if parsed == nil || parsed.To4() == nil {
		return "", "", "", fmt.Errorf("%w: public_ip must be a valid IPv4 address", ErrConfiguration)
	}
	return roomDomain, publicIP, baseDomain, nil
}

func normalizeDeleteInput(input DeleteRecordInput) (string, string, error) {
	roomDomain, err := normalizeDNSName(input.RoomDomain)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid room domain: %v", ErrConfiguration, err)
	}
	baseDomain, err := normalizeDNSName(input.BaseDomain)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid sessions base domain: %v", ErrConfiguration, err)
	}
	if !dnsNameWithin(roomDomain, baseDomain) || roomDomain == baseDomain {
		return "", "", fmt.Errorf("%w: room domain %s is not under sessions base domain %s", ErrConfiguration, roomDomain, baseDomain)
	}
	return roomDomain, baseDomain, nil
}

func normalizeDNSName(name string) (string, error) {
	name = strings.ToLower(strings.Trim(strings.TrimSpace(name), "."))
	if name == "" || len(name) > 253 {
		return "", fmt.Errorf("invalid DNS name length")
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return "", fmt.Errorf("invalid DNS label length")
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", fmt.Errorf("DNS labels must not start or end with a dash")
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return "", fmt.Errorf("DNS labels may contain only lowercase letters, numbers, and dashes")
		}
	}
	return name, nil
}

func zoneCandidates(baseDomain string) []string {
	labels := strings.Split(baseDomain, ".")
	candidates := make([]string, 0, len(labels)-1)
	for i := 0; i < len(labels)-1; i++ {
		candidates = append(candidates, strings.Join(labels[i:], "."))
	}
	return candidates
}

func dnsNameWithin(name string, suffix string) bool {
	return sameName(name, suffix) || strings.HasSuffix(name, "."+suffix)
}

func sameName(a string, b string) bool {
	return strings.EqualFold(strings.Trim(a, "."), strings.Trim(b, "."))
}

func exactARecord(record cloudflareRecord, roomDomain string) bool {
	return sameName(record.Name, roomDomain) && record.Type == "A"
}

func cloudflareError(operation string, messages []cloudflareAPIMessage) error {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if strings.TrimSpace(message.Message) != "" {
			parts = append(parts, fmt.Sprintf("%d %s", message.Code, message.Message))
		}
	}
	if len(parts) == 0 {
		return fmt.Errorf("cloudflare %s failed", operation)
	}
	return fmt.Errorf("cloudflare %s failed: %s", operation, strings.Join(parts, "; "))
}

func cloudflareMessagesNotFound(messages []cloudflareAPIMessage) bool {
	for _, message := range messages {
		text := strings.ToLower(message.Message)
		if message.Code == 1001 || strings.Contains(text, "not found") {
			return true
		}
	}
	return false
}

type notFoundError struct {
	op string
}

func (e notFoundError) Error() string {
	return "cloudflare " + e.op + " returned not found"
}

func isNotFoundError(err error) bool {
	_, ok := err.(notFoundError)
	return ok
}
