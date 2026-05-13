import { useState, type ReactNode } from "react";
import { Link, useLocation, useParams } from "react-router";
import { useForceDestroySessionServer, useSessionDetail } from "../api/hooks";
import { Alert } from "../components/Alert";
import { Icon } from "../components/Icon";
import { Region, regionLabel } from "../components/Region";
import { Shell } from "../components/Shell";
import { StatusBadge, statusIcon } from "../components/StatusBadge";
import {
  isAttentionStatus,
  isJoinRedirectReadyStatus,
  SESSION_LIFECYCLE_STATUSES,
  sessionLifecycleIndex,
  sessionStatusLabel,
  type SessionStatus,
} from "../domain/sessionStatus";
import type { AccessToken, CreateSessionResponse, Detail, Event, Session } from "../types";
import { messageFromError } from "../utils/errors";
import { formatDate, formatDateTime, formatTime } from "../utils/format";
import { domainFor, value } from "../utils/session";
import { NotFound } from "./NotFound";

export function SessionDetailPage() {
  const { id } = useParams();
  const location = useLocation();
  const created = isCreatedState(location.state) ? location.state.created : undefined;
  const detail = useSessionDetail(id);

  if (!id) return <NotFound />;
  return (
    <Shell active="sessions">
      <p className="back">
        <Link to={{ pathname: "/sessions", search: location.search }}>← Back to sessions</Link>
      </p>
      {detail.isLoading ? <p className="muted">Loading session…</p> : null}
      {detail.isError ? <Alert>{messageFromError(detail.error)}</Alert> : null}
      {detail.data ? <SessionDetail detail={detail.data} created={created} /> : null}
    </Shell>
  );
}

function SessionDetail({ detail, created }: { detail: Detail; created?: CreateSessionResponse }) {
  const session = detail.session;
  const hasRuntime = Boolean(session.instance_id || session.public_ip);
  const canOpenRoom = isJoinRedirectReadyStatus(session.status) && Boolean(session.room_domain);
  const canForceDestroy = ["provisioning", "waiting_for_dns", "failed"].includes(session.status);
  const [confirmation, setConfirmation] = useState("");
  const [forceDestroyOpen, setForceDestroyOpen] = useState(false);
  const forceDestroy = useForceDestroySessionServer({
    onSuccess: () => setForceDestroyOpen(false),
  });
  const expectedConfirmation = `destroy ${session.slug}`;
  return (
    <>
      <div className="detail-head">
        <div>
          <h1>{session.title}</h1>
          <p>
            <code>{session.id}</code> <StatusBadge status={session.status} />
          </p>
        </div>
        <div className="detail-actions">
          <button className="button danger" type="button">
            ⦿ End session
          </button>
          {canForceDestroy ? (
            <button
              className="button danger"
              type="button"
              onClick={() => setForceDestroyOpen(true)}
            >
              Force destroy session server
            </button>
          ) : null}
          {hasRuntime ? (
            <button className="button ghost" type="button">
              ↻ Retry health check
            </button>
          ) : null}
          {canOpenRoom ? (
            <a className="button primary" href={`https://${domainFor(session)}`}>
              ↗ Open room
            </a>
          ) : (
            <button className="button primary" disabled type="button">
              Room not ready
            </button>
          )}
        </div>
      </div>
      {created ? <CreatedLinks created={created} /> : null}
      <section className="panel info-grid">
        <Info label="Status">
          <StatusBadge status={session.status} />
        </Info>
        <Info label="Instance ID">{value(session.instance_id)}</Info>
        <Info label="Session ID">
          {session.id} <CopyMiniButton label="Copy session ID" value={session.id} />
        </Info>
        <Info label="Region">
          <Region region={session.instance_region} />
        </Info>
        <Info label="Public IP">
          {value(session.public_ip)}{" "}
          {session.public_ip ? (
            <CopyMiniButton label="Copy public IP" value={session.public_ip} />
          ) : null}
        </Info>
        <Info label="Live heartbeat">
          {session.last_heartbeat_at ? (
            <>
              <span className="healthy">● Healthy</span>
              <small>Last seen {formatTime(session.last_heartbeat_at)}</small>
            </>
          ) : (
            "—"
          )}
        </Info>
        <Info label="Room domain">{domainFor(session)}</Info>
        <Info label="Slug">{session.slug}</Info>
        <Info label="DNS record ID">{value(session.dns_record_id)}</Info>
        <Info label="Provision attempts">{session.provision_attempts}</Info>
        <Info label="Created">{formatDateTime(session.created_at)}</Info>
      </section>
      {forceDestroyOpen ? (
        <section className="panel attention space">
          <h2>Force destroy session server</h2>
          <p>
            This permanently deletes the DigitalOcean session server for this early lifecycle state.
            Type <code>{expectedConfirmation}</code> to continue.
          </p>
          {forceDestroy.isError ? <Alert>{messageFromError(forceDestroy.error)}</Alert> : null}
          <input
            aria-label="Force destroy confirmation"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
          />
          <div className="detail-actions">
            <button
              className="button ghost"
              type="button"
              onClick={() => setForceDestroyOpen(false)}
            >
              Cancel
            </button>
            <button
              aria-busy={forceDestroy.isPending}
              className="button danger"
              type="button"
              disabled={confirmation !== expectedConfirmation || forceDestroy.isPending}
              onClick={() => forceDestroy.mutate({ id: session.id, confirmation })}
            >
              {forceDestroy.isPending ? "Destroying…" : "Force destroy session server"}
            </button>
          </div>
        </section>
      ) : null}
      {session.last_error ? <FailureCard session={session} /> : null}
      {session.status === "awaiting_manual_download" ? (
        <ManualDownloadCard session={session} />
      ) : null}
      <Lifecycle status={session.status} />
      <div className="detail-grid">
        <main>
          <JoinLinks created={created} />
          <AccessTokensCard tokens={detail.access_tokens} />
          <EventsCard events={detail.events} />
        </main>
        <aside className="diagnostic-stack">
          {hasRuntime ? (
            <>
              <HealthCard
                title="LiveKit"
                icon="⌁"
                statusLabel={session.last_heartbeat_at ? "Healthy" : "Provisioned"}
                rows={[
                  ["Room", session.slug],
                  ["Region", regionLabel(session.instance_region)],
                  ["Uptime", session.last_heartbeat_at ? "2h 14m 32s" : "—"],
                ]}
              />
              <HealthCard
                title="Recording server"
                icon="⇩"
                statusLabel={session.status === "active" ? "Recording" : "Provisioned"}
                rows={[
                  ["Endpoint", session.public_ip ? `${session.public_ip}:7880` : "—"],
                  ["Status", session.status === "active" ? "Recording" : "Provisioned"],
                  ["Uptime", session.last_heartbeat_at ? "2h 14m 31s" : "—"],
                ]}
              />
              <HealthCard
                title="Disk"
                icon="▣"
                statusLabel="Unknown"
                rows={[
                  ["Usage", "—"],
                  ["Free space", "—"],
                  ["I/O", "Unknown"],
                ]}
              />
              <HealthCard
                title="DNS"
                icon="◎"
                statusLabel={session.dns_record_id ? "Configured" : "Pending"}
                rows={[
                  ["Domain", value(session.room_domain)],
                  ["A record", value(session.public_ip)],
                  ["TTL", session.dns_record_id ? "60s" : "—"],
                ]}
              />
            </>
          ) : (
            <ProvisioningDiagnostics session={session} />
          )}
        </aside>
      </div>
    </>
  );
}

function Info({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <span className="info-label">{label}</span>
      <p>{children}</p>
    </div>
  );
}

function CopyMiniButton({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    await navigator.clipboard.writeText(value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  return (
    <button
      aria-label={label}
      className="copy-mini"
      title={copied ? "Copied" : label}
      type="button"
      onClick={copy}
    >
      <Icon name="copy" />
    </button>
  );
}

function ProvisioningDiagnostics({ session }: { session: Session }) {
  return (
    <section className="panel health-card">
      <div className="health-head">
        <h2>
          <span>◎</span>
          Provisioning
        </h2>
        <b>{session.status === "failed" ? "Needs attention" : "Pending"}</b>
      </div>
      <div className="health-row">
        <span>Room server</span>
        <strong>Not provisioned</strong>
      </div>
      <div className="health-row">
        <span>Room domain</span>
        <strong>{value(session.room_domain)}</strong>
      </div>
    </section>
  );
}

function Lifecycle({ status }: { status: SessionStatus }) {
  const current = sessionLifecycleIndex(status);
  return (
    <section className="panel lifecycle">
      {isAttentionStatus(status) ? (
        <div className="life-alert">
          <StatusBadge status={status} />
          <span>
            Lifecycle stopped before successful completion. Check the error details and events.
          </span>
        </div>
      ) : null}
      {SESSION_LIFECYCLE_STATUSES.map((step, index) => {
        const isDone = current !== null && index < current;
        const isCurrent = current !== null && index === current;
        return (
          <div
            className={`life-step ${isDone ? "done" : ""} ${isCurrent ? "current" : ""}`}
            key={step}
          >
            <span>{isDone ? "✓" : isCurrent ? statusIcon(step) : ""}</span>
            <strong>{sessionStatusLabel(step)}</strong>
            <small>{current !== null && index <= current ? `10:2${index}:0${index} AM` : ""}</small>
          </div>
        );
      })}
    </section>
  );
}

function FailureCard({ session }: { session: Session }) {
  return (
    <section className="panel attention space">
      <h2>Failure details</h2>
      <p>{session.last_error}</p>
      <dl>
        <div>
          <dt>Phase</dt>
          <dd>{value(session.last_error_phase)}</dd>
        </div>
        <div>
          <dt>Last error</dt>
          <dd>{session.last_error_at ? formatDateTime(session.last_error_at) : "—"}</dd>
        </div>
      </dl>
    </section>
  );
}

function ManualDownloadCard({ session }: { session: Session }) {
  return (
    <section className="panel attention space">
      <h2>Manual download required</h2>
      <p>Download and verify the recording archive before confirming teardown.</p>
      {session.recording_download_url ? (
        <a href={session.recording_download_url}>Download recordings</a>
      ) : null}
    </section>
  );
}

function JoinLinks({ created }: { created?: CreateSessionResponse }) {
  if (!created) {
    return (
      <section className="panel join-links join-links-unavailable">
        <div>
          <span>Join links</span>
          <p className="muted">
            Raw host and guest join links are shown only when the session is created. Stored access
            tokens below show metadata only; token plaintext cannot be recovered.
          </p>
        </div>
      </section>
    );
  }

  return (
    <section className="panel join-links">
      <LinkLine label="Host join link" url={created.join_links.host.url} />
      <LinkLine label="Guest join link" url={created.join_links.guest.url} />
    </section>
  );
}

function LinkLine({ label, url }: { label: string; url: string }) {
  return (
    <div>
      <span>{label}</span>
      <code>{url}</code>
      <button className="button ghost" type="button">
        □ Copy
      </button>
    </div>
  );
}

function AccessTokensCard({ tokens }: { tokens: Detail["access_tokens"] }) {
  return (
    <section className="panel access-token-card">
      <div className="section-head">
        <h2>Access tokens</h2>
      </div>
      {tokens.length === 0 ? (
        <p className="muted">No persisted access token metadata yet.</p>
      ) : (
        <ul>
          {tokens.map((token) => (
            <li className={token.revoked_at ? "token-revoked" : undefined} key={token.id}>
              <span className="event-line" />
              <b>{token.role}</b>
              <p>
                {token.label ?? "Unlabeled token"}
                {token.revoked_at ? " · revoked" : " · active"}
              </p>
              <time>{tokenUsageLabel(token)}</time>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function tokenUsageLabel(token: AccessToken) {
  if (token.revoked_at) {
    return `Revoked ${formatDate(token.revoked_at)} ${formatTime(token.revoked_at)}`;
  }
  if (token.last_used_at) {
    return `Last used ${formatDate(token.last_used_at)} ${formatTime(token.last_used_at)}`;
  }
  return "Never used";
}

function EventsCard({ events }: { events: Event[] }) {
  return (
    <section className="panel events-card">
      <div className="section-head">
        <h2>Session events</h2>
        <button className="button ghost" type="button">
          ⇩ Download
        </button>
      </div>
      {events.length === 0 ? (
        <p className="muted">No session events recorded yet.</p>
      ) : (
        <ul>
          {events.slice(0, 10).map((event) => (
            <li key={event.id}>
              <time>{formatTime(event.created_at)}</time>
              <span className="event-line" />
              <b>{event.type}</b>
              <p>{value(event.message)}</p>
              <time>{formatDate(event.created_at)}</time>
            </li>
          ))}
        </ul>
      )}
      {events.length > 10 ? (
        <button className="button ghost load-more" type="button">
          Load more events ↓
        </button>
      ) : null}
    </section>
  );
}

function HealthCard({
  title,
  icon,
  rows,
  progress = false,
  statusLabel = "Observed",
}: {
  title: string;
  icon: string;
  rows: string[][];
  progress?: boolean;
  statusLabel?: string;
}) {
  return (
    <section className="panel health-card">
      <div className="health-head">
        <h2>
          <span>{icon}</span>
          {title}
        </h2>
        <b>{statusLabel}</b>
      </div>
      {rows.map(([key, val]) => (
        <div className="health-row" key={key}>
          <span>{key}</span>
          <strong>{val}</strong>
        </div>
      ))}
      {progress ? (
        <div className="meter">
          <span />
        </div>
      ) : null}
    </section>
  );
}

function CreatedLinks({ created }: { created: CreateSessionResponse }) {
  return (
    <section className="panel attention space">
      <h2>Session created</h2>
      <p>Raw join links are shown once. Store them now; SQLite keeps only token hashes.</p>
      <div className="links">
        <LinkBox label="Host" url={created.join_links.host.url} />
        <LinkBox label="Guest" url={created.join_links.guest.url} />
      </div>
    </section>
  );
}

function LinkBox({ label, url }: { label: string; url: string }) {
  return (
    <div className="linkbox">
      <strong>{label}</strong>
      <code>{url}</code>
    </div>
  );
}

function isCreatedState(state: unknown): state is { created: CreateSessionResponse } {
  return Boolean(state && typeof state === "object" && "created" in state);
}
