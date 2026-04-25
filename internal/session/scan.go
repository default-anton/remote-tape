package session

import (
	"database/sql"
	"fmt"
	"strings"
)

const sessionColumns = `
id, slug, title, status,
droplet_id, droplet_ip, droplet_region, droplet_size, image_id,
room_domain, dns_record_id, livekit_url,
recording_download_url, finalization_summary_json,
created_at, updated_at, ready_at, active_at, finalization_started_at, finalized_at,
last_heartbeat_at, download_confirmed_at, download_confirmed_by, ended_at, expires_at,
last_error, last_error_at, last_error_phase,
provision_attempts, dns_attempts, health_attempts, teardown_attempts`

type rowScanner interface {
	Scan(...any) error
}

func prefixedSessionColumns(prefix string) string {
	columns := strings.Split(strings.ReplaceAll(sessionColumns, "\n", " "), ",")
	for i, column := range columns {
		columns[i] = prefix + "." + strings.TrimSpace(column)
	}
	return strings.Join(columns, ", ")
}

func scanSession(row rowScanner) (Session, error) {
	var s Session
	var dropletID, dropletIP, roomDomain, dnsRecordID, liveKitURL sql.NullString
	var recordingDownloadURL, finalizationSummaryJSON sql.NullString
	var readyAt, activeAt, finalizationStartedAt, finalizedAt, lastHeartbeatAt sql.NullString
	var downloadConfirmedAt, downloadConfirmedBy, endedAt, expiresAt sql.NullString
	var lastError, lastErrorAt, lastErrorPhase sql.NullString
	if err := row.Scan(
		&s.ID,
		&s.Slug,
		&s.Title,
		&s.Status,
		&dropletID,
		&dropletIP,
		&s.DropletRegion,
		&s.DropletSize,
		&s.ImageID,
		&roomDomain,
		&dnsRecordID,
		&liveKitURL,
		&recordingDownloadURL,
		&finalizationSummaryJSON,
		&s.CreatedAt,
		&s.UpdatedAt,
		&readyAt,
		&activeAt,
		&finalizationStartedAt,
		&finalizedAt,
		&lastHeartbeatAt,
		&downloadConfirmedAt,
		&downloadConfirmedBy,
		&endedAt,
		&expiresAt,
		&lastError,
		&lastErrorAt,
		&lastErrorPhase,
		&s.ProvisionAttempts,
		&s.DNSAttempts,
		&s.HealthAttempts,
		&s.TeardownAttempts,
	); err != nil {
		return Session{}, fmt.Errorf("scan session: %w", err)
	}
	s.DropletID = nullStringPtr(dropletID)
	s.DropletIP = nullStringPtr(dropletIP)
	s.RoomDomain = nullStringPtr(roomDomain)
	s.DNSRecordID = nullStringPtr(dnsRecordID)
	s.LiveKitURL = nullStringPtr(liveKitURL)
	s.RecordingDownloadURL = nullStringPtr(recordingDownloadURL)
	s.FinalizationSummaryJSON = nullStringPtr(finalizationSummaryJSON)
	s.ReadyAt = nullStringPtr(readyAt)
	s.ActiveAt = nullStringPtr(activeAt)
	s.FinalizationStartedAt = nullStringPtr(finalizationStartedAt)
	s.FinalizedAt = nullStringPtr(finalizedAt)
	s.LastHeartbeatAt = nullStringPtr(lastHeartbeatAt)
	s.DownloadConfirmedAt = nullStringPtr(downloadConfirmedAt)
	s.DownloadConfirmedBy = nullStringPtr(downloadConfirmedBy)
	s.EndedAt = nullStringPtr(endedAt)
	s.ExpiresAt = nullStringPtr(expiresAt)
	s.LastError = nullStringPtr(lastError)
	s.LastErrorAt = nullStringPtr(lastErrorAt)
	s.LastErrorPhase = nullStringPtr(lastErrorPhase)
	return s, nil
}

func scanAccessToken(row rowScanner) (AccessToken, error) {
	var t AccessToken
	var label, lastUsedAt, revokedAt sql.NullString
	if err := row.Scan(&t.ID, &t.SessionID, &t.Role, &label, &t.CreatedAt, &lastUsedAt, &revokedAt); err != nil {
		return AccessToken{}, fmt.Errorf("scan access token: %w", err)
	}
	t.Label = nullStringPtr(label)
	t.LastUsedAt = nullStringPtr(lastUsedAt)
	t.RevokedAt = nullStringPtr(revokedAt)
	return t, nil
}

func scanEvent(row rowScanner) (Event, error) {
	var e Event
	var message, metadataJSON sql.NullString
	if err := row.Scan(&e.ID, &e.SessionID, &e.Type, &message, &metadataJSON, &e.CreatedAt); err != nil {
		return Event{}, fmt.Errorf("scan session event: %w", err)
	}
	e.Message = nullStringPtr(message)
	e.MetadataJSON = nullStringPtr(metadataJSON)
	return e, nil
}

func scanSessionAndToken(row rowScanner) (Session, AccessToken, error) {
	var s Session
	var t AccessToken
	var dropletID, dropletIP, roomDomain, dnsRecordID, liveKitURL sql.NullString
	var recordingDownloadURL, finalizationSummaryJSON sql.NullString
	var readyAt, activeAt, finalizationStartedAt, finalizedAt, lastHeartbeatAt sql.NullString
	var downloadConfirmedAt, downloadConfirmedBy, endedAt, expiresAt sql.NullString
	var lastError, lastErrorAt, lastErrorPhase sql.NullString
	var label, lastUsedAt, revokedAt sql.NullString
	if err := row.Scan(
		&s.ID,
		&s.Slug,
		&s.Title,
		&s.Status,
		&dropletID,
		&dropletIP,
		&s.DropletRegion,
		&s.DropletSize,
		&s.ImageID,
		&roomDomain,
		&dnsRecordID,
		&liveKitURL,
		&recordingDownloadURL,
		&finalizationSummaryJSON,
		&s.CreatedAt,
		&s.UpdatedAt,
		&readyAt,
		&activeAt,
		&finalizationStartedAt,
		&finalizedAt,
		&lastHeartbeatAt,
		&downloadConfirmedAt,
		&downloadConfirmedBy,
		&endedAt,
		&expiresAt,
		&lastError,
		&lastErrorAt,
		&lastErrorPhase,
		&s.ProvisionAttempts,
		&s.DNSAttempts,
		&s.HealthAttempts,
		&s.TeardownAttempts,
		&t.ID,
		&t.SessionID,
		&t.Role,
		&label,
		&t.CreatedAt,
		&lastUsedAt,
		&revokedAt,
	); err != nil {
		return Session{}, AccessToken{}, fmt.Errorf("scan join session: %w", err)
	}
	s.DropletID = nullStringPtr(dropletID)
	s.DropletIP = nullStringPtr(dropletIP)
	s.RoomDomain = nullStringPtr(roomDomain)
	s.DNSRecordID = nullStringPtr(dnsRecordID)
	s.LiveKitURL = nullStringPtr(liveKitURL)
	s.RecordingDownloadURL = nullStringPtr(recordingDownloadURL)
	s.FinalizationSummaryJSON = nullStringPtr(finalizationSummaryJSON)
	s.ReadyAt = nullStringPtr(readyAt)
	s.ActiveAt = nullStringPtr(activeAt)
	s.FinalizationStartedAt = nullStringPtr(finalizationStartedAt)
	s.FinalizedAt = nullStringPtr(finalizedAt)
	s.LastHeartbeatAt = nullStringPtr(lastHeartbeatAt)
	s.DownloadConfirmedAt = nullStringPtr(downloadConfirmedAt)
	s.DownloadConfirmedBy = nullStringPtr(downloadConfirmedBy)
	s.EndedAt = nullStringPtr(endedAt)
	s.ExpiresAt = nullStringPtr(expiresAt)
	s.LastError = nullStringPtr(lastError)
	s.LastErrorAt = nullStringPtr(lastErrorAt)
	s.LastErrorPhase = nullStringPtr(lastErrorPhase)
	t.Label = nullStringPtr(label)
	t.LastUsedAt = nullStringPtr(lastUsedAt)
	t.RevokedAt = nullStringPtr(revokedAt)
	return s, t, nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
