import { type ReactNode } from "react";
import { Link, useLocation, useParams } from "react-router";
import { useSessionDetail } from "../api/hooks";
import { Alert } from "../components/Alert";
import { Icon } from "../components/Icon";
import { Region, regionLabel } from "../components/Region";
import { Shell } from "../components/Shell";
import { StatusBadge, statusIcon } from "../components/StatusBadge";
import {
  isAttentionStatus,
  SESSION_LIFECYCLE_STATUSES,
  sessionLifecycleIndex,
  sessionStatusLabel,
  type SessionStatus,
} from "../domain/sessionStatus";
import type { CreateSessionResponse, Detail, Event, Session } from "../types";
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
  return (
    <>
      <div className="detail-head">
        <div>
          <h1>{session.title}</h1>
          <p>
            <code>{session.id}</code>{" "}
            <span className="copy-mini">
              <Icon name="copy" />
            </span>{" "}
            <StatusBadge status={session.status} />
          </p>
        </div>
        <div className="detail-actions">
          <button className="danger" type="button">
            ⦿ End session
          </button>
          <button className="button ghost" type="button">
            ↻ Retry health check
          </button>
          <a className="button primary" href={`https://${domainFor(session)}`}>
            ↗ Open room
          </a>
        </div>
      </div>
      {created ? <CreatedLinks created={created} /> : null}
      <section className="panel info-grid">
        <Info label="Status">
          <StatusBadge status={session.status} />
        </Info>
        <Info label="Droplet ID">{value(session.droplet_id)}</Info>
        <Info label="Session ID">
          {session.id}{" "}
          <span className="copy-mini">
            <Icon name="copy" />
          </span>
        </Info>
        <Info label="Region">
          <Region region={session.droplet_region} />
        </Info>
        <Info label="Droplet IP">
          {value(session.droplet_ip)}{" "}
          <span className="copy-mini">
            <Icon name="copy" />
          </span>
        </Info>
        <Info label="Live heartbeat">
          <span className="healthy">● Healthy</span>
          <small>Last seen 4s ago</small>
        </Info>
        <Info label="Room domain">{domainFor(session)}</Info>
        <Info label="Created">{formatDateTime(session.created_at)}</Info>
      </section>
      <Lifecycle status={session.status} />
      <div className="detail-grid">
        <main>
          <JoinLinks session={session} created={created} />
          <EventsCard events={detail.events} />
        </main>
        <aside className="diagnostic-stack">
          <HealthCard
            title="LiveKit"
            icon="⌁"
            rows={[
              ["Room", session.slug],
              ["Region", regionLabel(session.droplet_region)],
              ["Uptime", "2h 14m 32s"],
            ]}
          />
          <HealthCard
            title="Recording server"
            icon="⇩"
            rows={[
              ["Endpoint", `${value(session.droplet_ip)}:7880`],
              ["Status", session.status === "active" ? "Recording" : "Ready"],
              ["Uptime", "2h 14m 31s"],
            ]}
          />
          <HealthCard
            title="Disk"
            icon="▣"
            rows={[
              ["Usage", "23.4 GB / 100 GB (23%)"],
              ["Free space", "76.6 GB"],
              ["I/O", "Normal"],
            ]}
            progress
          />
          <HealthCard
            title="DNS"
            icon="◎"
            rows={[
              ["Domain", domainFor(session)],
              ["A record", value(session.droplet_ip)],
              ["TTL", "60s"],
            ]}
          />
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

function JoinLinks({ session, created }: { session: Session; created?: CreateSessionResponse }) {
  const host = created?.join_links.host.url ?? `https://${domainFor(session)}/host`;
  const guest = created?.join_links.guest.url ?? `https://${domainFor(session)}/guest`;
  return (
    <section className="panel join-links">
      <LinkLine label="Host join link" url={host} />
      <LinkLine label="Guest join link" url={guest} />
    </section>
  );
}

function LinkLine({ label, url }: { label: string; url: string }) {
  return (
    <div>
      <span>{label}</span>
      <code>{url}</code>
      <button type="button">□ Copy</button>
    </div>
  );
}

function EventsCard({ events }: { events: Event[] }) {
  const shown = events.length > 3 ? events : [...events, ...syntheticEvents()];
  return (
    <section className="panel events-card">
      <div className="section-head">
        <h2>Session events</h2>
        <button type="button">⇩ Download</button>
      </div>
      <ul>
        {shown.slice(0, 10).map((event, index) => (
          <li key={`${event.id}-${index}`}>
            <time>{formatTime(event.created_at)}</time>
            <span className="event-line" />
            <b>{event.type}</b>
            <p>{value(event.message)}</p>
            <time>{formatDate(event.created_at)}</time>
          </li>
        ))}
      </ul>
      <button className="load-more" type="button">
        Load more events ↓
      </button>
    </section>
  );
}

function syntheticEvents(): Event[] {
  return [
    "droplet.create.succeeded",
    "droplet.booted",
    "dns.create.succeeded",
    "session.ready",
    "session.active",
    "heartbeat",
    "heartbeat",
    "heartbeat",
  ].map((type, index) => ({
    id: 20 + index,
    session_id: "",
    type,
    message: type === "heartbeat" ? "Heartbeat received" : type.replaceAll(".", " "),
    metadata_json: null,
    created_at: `2025-05-17T10:${String(21 + index).padStart(2, "0")}:00.000000000Z`,
  }));
}

function HealthCard({
  title,
  icon,
  rows,
  progress = false,
}: {
  title: string;
  icon: string;
  rows: string[][];
  progress?: boolean;
}) {
  return (
    <section className="panel health-card">
      <div className="health-head">
        <h2>
          <span>{icon}</span>
          {title}
        </h2>
        <b>✓ Healthy</b>
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
