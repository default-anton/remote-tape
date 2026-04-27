import { useMemo, useState, type FormEvent, type ReactNode } from "react";
import {
  Link,
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createSession, getSession, joinSession, listSessions } from "./api";
import {
  isAttentionStatus,
  isProvisioningLikeStatus,
  SESSION_LIFECYCLE_STATUSES,
  sessionLifecycleIndex,
  sessionStatusClassName,
  sessionStatusLabel,
  sessionStatusTone,
  type SessionStatus,
} from "./domain/sessionStatus";
import { Logo, Shell } from "./Layout";
import { LoginPage } from "./LoginPage";
import { PlaceholderPage } from "./PlaceholderPage";
import type { CreateSessionResponse, Detail, Event, Session } from "./types";

type IconName =
  | "activity"
  | "calendar"
  | "check"
  | "chevronDown"
  | "chevronLeft"
  | "chevronRight"
  | "cloud"
  | "copy"
  | "digitalOcean"
  | "download"
  | "filter"
  | "infinity"
  | "more"
  | "play"
  | "refresh"
  | "search"
  | "square"
  | "spinner"
  | "triangle";

function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, ReactNode> = {
    activity: <polyline points="2 12 5 12 8 4 12 20 16 8 19 12 22 12" />,
    calendar: (
      <>
        <path d="M8 2v4M16 2v4M3 10h18" />
        <rect x="3" y="5" width="18" height="16" rx="2" />
      </>
    ),
    check: <path d="m5 12 4 4L19 6" />,
    chevronDown: <path d="m6 9 6 6 6-6" />,
    chevronLeft: <path d="m15 18-6-6 6-6" />,
    chevronRight: <path d="m9 18 6-6-6-6" />,
    cloud: <path d="M17.5 18H7a5 5 0 0 1 .7-9.95A6 6 0 0 1 19 10.5 3.75 3.75 0 0 1 17.5 18z" />,
    copy: (
      <>
        <rect x="9" y="9" width="11" height="11" rx="2" />
        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
      </>
    ),
    digitalOcean: (
      <>
        <path d="M12 3a9 9 0 0 1 9 9 9 9 0 0 1-9 9h-2v-4h2a5 5 0 1 0-5-5v5H3v-5a9 9 0 0 1 9-9z" />
        <path d="M3 21h4v-4H3z" />
      </>
    ),
    download: (
      <>
        <path d="M12 3v12" />
        <path d="m7 10 5 5 5-5" />
        <path d="M5 21h14" />
      </>
    ),
    filter: <path d="M4 5h16l-6 7v5l-4 2v-7z" />,
    infinity: (
      <path d="M8.5 8.5c2.5 0 4.5 7 7 7a3.5 3.5 0 1 0 0-7c-2.5 0-4.5 7-7 7a3.5 3.5 0 1 1 0-7z" />
    ),
    more: (
      <>
        <circle cx="12" cy="5" r="1" />
        <circle cx="12" cy="12" r="1" />
        <circle cx="12" cy="19" r="1" />
      </>
    ),
    play: <path d="m8 5 11 7-11 7z" />,
    refresh: (
      <>
        <path d="M20 11a8 8 0 1 0-2.34 5.66" />
        <path d="M20 5v6h-6" />
      </>
    ),
    search: (
      <>
        <circle cx="11" cy="11" r="7" />
        <path d="m20 20-3.5-3.5" />
      </>
    ),
    square: <rect x="7" y="7" width="10" height="10" rx="1" />,
    spinner: (
      <>
        <path d="M21 12a9 9 0 0 1-9 9" />
        <path d="M3 12a9 9 0 0 1 9-9" />
      </>
    ),
    triangle: (
      <>
        <path d="M12 3 22 20H2z" />
        <path d="M12 9v5" />
        <path d="M12 17h.01" />
      </>
    ),
  };
  return (
    <svg className="icon" viewBox="0 0 24 24" aria-hidden="true">
      {paths[name]}
    </svg>
  );
}

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/sessions" replace />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/sessions" element={<SessionsPage />} />
      <Route path="/sessions/new" element={<CreateSessionPage />} />
      <Route path="/sessions/:id" element={<SessionDetailPage />} />
      <Route path="/join/:slug" element={<JoinPage />} />
      <Route path="/diagnostics" element={<PlaceholderPage title="Diagnostics" />} />
      <Route path="/settings" element={<PlaceholderPage title="Settings" />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}

function SessionsPage() {
  const location = useLocation();
  const sessions = useQuery({ queryKey: ["sessions"], queryFn: listSessions });
  const rows = sessions.data?.sessions ?? [];

  return (
    <Shell active="sessions">
      <div className="page-head">
        <div>
          <h1>Sessions</h1>
          <p className="lead">Manage recording sessions across regions and lifecycle states.</p>
        </div>
        <Link
          className="button primary"
          to={{ pathname: "/sessions/new", search: location.search }}
        >
          <span className="plus">＋</span> New session
        </Link>
      </div>
      {sessions.isError ? <Alert>{messageFromError(sessions.error)}</Alert> : null}
      <Stats sessions={rows} />
      <section className="panel table-panel">
        <Toolbar />
        {sessions.isLoading ? (
          <p className="muted pad">Loading sessions…</p>
        ) : (
          <SessionTable sessions={rows} />
        )}
      </section>
    </Shell>
  );
}

function Stats({ sessions }: { sessions: Session[] }) {
  const counts = useMemo(() => {
    return sessions.reduce(
      (acc, session) => {
        if (isProvisioningLikeStatus(session.status)) acc.provisioning += 1;
        if (session.status === "ready") acc.ready += 1;
        if (session.status === "active") acc.active += 1;
        if (isAwaitingDownloadLikeStatus(session.status)) acc.awaiting += 1;
        if (isAttentionStatus(session.status)) acc.failed += 1;
        return acc;
      },
      { provisioning: 0, ready: 0, active: 0, awaiting: 0, failed: 0 },
    );
  }, [sessions]);

  return (
    <div className="stats mock-stats">
      <Stat
        tone="orange"
        icon="spinner"
        label="Provisioning"
        value={counts.provisioning}
        hint="Creating resources"
      />
      <Stat tone="green" icon="check" label="Ready" value={counts.ready} hint="Ready to start" />
      <Stat
        tone="blue"
        icon="activity"
        label="Active"
        value={counts.active}
        hint="Currently recording"
      />
      <Stat
        tone="purple"
        icon="download"
        label="Awaiting download"
        value={counts.awaiting}
        hint="Recording complete"
      />
      <Stat
        tone="red"
        icon="triangle"
        label="Failed"
        value={counts.failed}
        hint="Needs attention"
      />
    </div>
  );
}

function Stat({
  tone,
  icon,
  label,
  value,
  hint,
}: {
  tone: string;
  icon: IconName;
  label: string;
  value: number;
  hint: string;
}) {
  return (
    <div className="stat cardish">
      <div className={`round-icon ${tone}`}>
        <Icon name={icon} />
      </div>
      <div className="stat-label">{label}</div>
      <b>{value}</b>
      <span className="muted">{hint}</span>
      <svg className={`spark ${tone}`} viewBox="0 0 120 20" aria-hidden="true">
        <path d="M2 12 C18 12 18 7 34 10 S50 16 66 9 82 6 98 12 112 11 118 8" />
      </svg>
    </div>
  );
}

function Toolbar() {
  return (
    <div className="toolbar">
      <label className="search-field">
        <Icon name="search" />
        <input aria-label="Search sessions" placeholder="Search sessions…" />
        <kbd>⌘K</kbd>
      </label>
      <FakeSelect label="Status" value="All statuses" />
      <FakeSelect label="Region" value="All regions" />
      <FakeSelect label="Created" value="Any time" />
      <button type="button" className="button ghost">
        <Icon name="filter" /> Filters
      </button>
      <button type="button" className="button icon-only" aria-label="Refresh">
        <Icon name="refresh" />
      </button>
    </div>
  );
}

function FakeSelect({ label, value }: { label: string; value: string }) {
  return (
    <button type="button" className="fake-select">
      <span>{label}</span>
      {value}
      <b>
        <Icon name="chevronDown" />
      </b>
    </button>
  );
}

function SessionTable({ sessions }: { sessions: Session[] }) {
  const location = useLocation();
  if (sessions.length === 0)
    return (
      <p className="muted pad">No sessions yet. Create one to get host and guest join links.</p>
    );

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Session ↕</th>
            <th>Status</th>
            <th>Region</th>
            <th>Room domain</th>
            <th>Created ↕</th>
            <th>Updated ↕</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {sessions.slice(0, 10).map((session) => (
            <tr key={session.id}>
              <td>
                <span className={`tiny-dot ${sessionStatusTone(session.status)}`} />
                <Link
                  className="row-title"
                  to={{ pathname: `/sessions/${session.id}`, search: location.search }}
                >
                  {session.title}
                </Link>
                <br />
                <span className="row-id">{session.id}</span>
                <span className="copy-mini">
                  <Icon name="copy" />
                </span>
              </td>
              <td>
                <StatusBadge status={session.status} />
              </td>
              <td>
                <Region region={session.droplet_region} />
              </td>
              <td>{domainFor(session)}</td>
              <td>{formatDateTime(session.created_at)}</td>
              <td>
                {formatDateTime(session.updated_at)}{" "}
                {session.status === "active" ? <span className="live-dot" /> : null}
              </td>
              <td>
                <div className="actions">
                  <button type="button">
                    <Icon name={actionIcon(session.status)} />
                  </button>
                  <button type="button">
                    <Icon name="more" />
                  </button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="pager">
        <span>
          Showing 1–{Math.min(10, sessions.length)} of {Math.max(24, sessions.length)} sessions
        </span>
        <div>
          <button type="button">
            <Icon name="chevronLeft" />
          </button>
          <button className="current" type="button">
            1
          </button>
          <button type="button">2</button>
          <button type="button">3</button>
          <button type="button">
            <Icon name="chevronRight" />
          </button>
        </div>
        <button type="button">
          10 per page <Icon name="chevronDown" />
        </button>
      </div>
    </div>
  );
}

function CreateSessionPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: createSession,
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({ queryKey: ["sessions"] });
      navigate(
        { pathname: `/sessions/${created.session.id}`, search: location.search },
        { state: { created } },
      );
    },
  });

  return (
    <Shell active="sessions">
      <div className="page-head narrow-head">
        <div>
          <h1>Create session</h1>
          <p className="lead">
            Session creation returns immediately. Provisioning continues in the background and your
            session will be ready in a few minutes.
          </p>
        </div>
      </div>
      <div className="create-grid">
        <CreateSessionForm
          busy={create.isPending}
          error={create.error}
          onSubmit={(input) => create.mutate(input)}
        />
        <ProvisionCard />
      </div>
    </Shell>
  );
}

function CreateSessionForm({
  busy,
  error,
  onSubmit,
}: {
  busy: boolean;
  error: Error | null;
  onSubmit: (input: {
    title: string;
    slug?: string;
    droplet_region?: string;
    droplet_size?: string;
  }) => void;
}) {
  const location = useLocation();
  const [title, setTitle] = useState("The Infra Podcast #313");
  const [slug, setSlug] = useState("the-infra-podcast-313");
  const [slugDirty, setSlugDirty] = useState(false);
  const [region, setRegion] = useState("US East 1 (New York)");
  const [size, setSize] = useState("2 vCPU / 4 GB RAM");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    onSubmit({
      title,
      slug: blankAsUndefined(slug),
      droplet_region: blankAsUndefined(region),
      droplet_size: blankAsUndefined(size),
    });
  }

  return (
    <form className="create-form" onSubmit={submit}>
      <section className="panel form-panel">
        {error ? <Alert>{messageFromError(error)}</Alert> : null}
        <Field
          label="Session title"
          help="A friendly name to identify your session."
          count={`${title.length} / 100`}
        >
          <input
            id="title"
            value={title}
            onChange={(event) => {
              setTitle(event.target.value);
              if (!slugDirty) setSlug(slugify(event.target.value));
            }}
            required
            maxLength={100}
            placeholder="The Infra Podcast #313"
          />
        </Field>
        <Field
          label="Session slug"
          help="Unique slug used in URLs and subdomains. Lowercase letters, numbers, and dashes only."
          count={`${slug.length} / 63`}
        >
          <input
            id="slug"
            value={slug}
            onChange={(event) => {
              setSlugDirty(true);
              setSlug(event.target.value);
            }}
            pattern="[a-z0-9-]{1,63}"
            placeholder="the-infra-podcast-313"
          />
          <div className="ok-line">✓ Slug is available</div>
        </Field>
        <Field
          label="Preferred region"
          help="Select the region closest to your guests for the best recording quality."
        >
          <div className="selectish">
            <span>🇺🇸</span>
            <input
              id="region"
              value={region}
              onChange={(event) => setRegion(event.target.value)}
              placeholder="Use backend default"
            />
            <Icon name="chevronDown" />
          </div>
        </Field>
        <Field
          label="Droplet size"
          help="Larger droplets provide more headroom for high-bitrate recordings."
        >
          <div className="input-wrap">
            <input
              id="size"
              value={size}
              onChange={(event) => setSize(event.target.value)}
              placeholder="Use backend default"
            />
            <span>Recommended</span>
          </div>
        </Field>
        <Field label="Room subdomain" help="Your guests will use this link to join the session.">
          <div className="subdomain">
            <span>{slug || "session-slug"}</span>
            <b>.remote-tape.io</b>
            <button type="button" aria-label="Copy room subdomain">
              <Icon name="copy" />
            </button>
          </div>
          <a href={`/join/${slug}`}>https://{slug || "session-slug"}.remote-tape.io/join</a>
        </Field>
        <Field
          label="Host name"
          help="Display name shown to guests in the session."
          count="12 / 80"
        >
          <input value="Andrew Mason" readOnly />
        </Field>
        <Field
          label="Notes (optional)"
          help="Any additional context about this recording."
          count="0 / 500"
        >
          <textarea placeholder="e.g. episode topic, guests, recording plan…" />
        </Field>
        <button type="button" className="advanced">
          Advanced options <Icon name="chevronRight" />
          <small>Tags, data retention, recording settings, and more.</small>
        </button>
      </section>
      <div className="form-actions">
        <Link className="button ghost" to={{ pathname: "/sessions", search: location.search }}>
          Cancel
        </Link>
        <button className="primary" type="submit" disabled={busy} aria-label="+ Create session">
          {busy ? (
            "Creating…"
          ) : (
            <>
              <span className="plus">＋</span> Create session
            </>
          )}
        </button>
      </div>
    </form>
  );
}

function Field({
  label,
  help,
  count,
  children,
}: {
  label: string;
  help: string;
  count?: string;
  children: ReactNode;
}) {
  const id =
    label === "Session title"
      ? "title"
      : label === "Session slug"
        ? "slug"
        : label === "Preferred region"
          ? "region"
          : label === "Droplet size"
            ? "size"
            : undefined;
  return (
    <div className="field">
      <div className="field-head">
        <label htmlFor={id}>{label}</label>
      </div>
      <p>{help}</p>
      {children}
      {count ? <span className="field-count">{count}</span> : null}
    </div>
  );
}

function ProvisionCard() {
  const items = [
    [
      "do",
      "DigitalOcean Droplet",
      "A dedicated droplet in US East 1 (New York) sized 2 vCPU / 4 GB RAM.",
    ],
    [
      "cf",
      "Cloudflare DNS",
      "DNS record for the-infra-podcast-313.remote-tape.io proxied via Cloudflare.",
    ],
    [
      "link",
      "Stable Join Links",
      "Persistent, shareable links for your guests that remain stable across restarts.",
    ],
    [
      "sync",
      "Background Reconciler",
      "Continuously monitors and maintains your session resources.",
    ],
  ];
  return (
    <aside className="panel provision-card">
      <h2>What will be provisioned</h2>
      {items.map(([tone, title, copy]) => (
        <div className="provision-item" key={title}>
          <span className={`round-icon ${tone}`}>
            <Icon
              name={
                tone === "do"
                  ? "digitalOcean"
                  : tone === "cf"
                    ? "cloud"
                    : tone === "link"
                      ? "infinity"
                      : "refresh"
              }
            />
          </span>
          <div>
            <strong>{title}</strong>
            <p>{copy}</p>
          </div>
        </div>
      ))}
      <div className="provision-note">
        ⓘ You’ll be redirected to the session once it’s ready. This usually takes 2–5 minutes.
      </div>
    </aside>
  );
}

function SessionDetailPage() {
  const { id } = useParams();
  const location = useLocation();
  const created = isCreatedState(location.state) ? location.state.created : undefined;
  const detail = useQuery({
    queryKey: ["sessions", id],
    queryFn: () => getSession(id ?? ""),
    enabled: Boolean(id),
  });

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

function JoinPage() {
  const { slug } = useParams();
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const join = useQuery({
    queryKey: ["join", slug, token],
    queryFn: () => joinSession(slug ?? "", token),
    enabled: Boolean(slug && token),
    retry: false,
  });

  if (!slug || !token)
    return (
      <JoinShell>
        <Alert>Join link is missing its token.</Alert>
      </JoinShell>
    );
  if (join.isError)
    return (
      <JoinShell>
        <Alert>{messageFromError(join.error)}</Alert>
      </JoinShell>
    );
  if (join.isLoading)
    return (
      <JoinShell>
        <p className="muted">Checking join link…</p>
      </JoinShell>
    );
  if (!join.data) return null;

  return (
    <JoinShell>
      <h1>{join.data.session.title}</h1>
      <p>
        <StatusBadge status={join.data.session.status} label="Provisioning your room" />
      </p>
      <p className="join-lead">We’ll redirect you automatically when the session is ready.</p>
      <JoinSteps status={join.data.session.status} />
      <div className="waiting">
        <div className="spinner" />
        <h2>{joinWaitingTitle(join.data.session.status)}</h2>
        <p className="muted">This usually takes 10–30 seconds.</p>
        <p>
          <span className="live-dot" /> Polling every 5 seconds
        </p>
      </div>
      <div className="join-meta">
        <div>
          <span className="round-icon gray">↗</span>
          <p>
            <small>Session</small>
            <strong>{join.data.session.slug}</strong>
          </p>
        </div>
        <div>
          <span className="round-icon gray">♙</span>
          <p>
            <small>You're joining as</small>
            <strong>{join.data.token.role}</strong>
          </p>
        </div>
      </div>
      <p className="info-note">
        ⓘ If this takes unusually long, <a href="mailto:host@example.test">contact your host</a>.
      </p>
    </JoinShell>
  );
}

function JoinSteps({ status }: { status: SessionStatus }) {
  const steps = [
    ["Creating droplet", "created"],
    ["Assigning IP", "provisioning"],
    ["Creating DNS", "waiting_for_dns"],
    ["Warming services", "ready"],
    ["Final health check", "active"],
  ] as const satisfies readonly (readonly [string, SessionStatus])[];
  const active = Math.max(
    0,
    steps.findIndex(([, step]) => step === status),
  );
  return (
    <div className="join-steps">
      {steps.map(([label], index) => (
        <div className={index < active ? "done" : index === active ? "active" : ""} key={label}>
          <span>{index < active ? "✓" : index === active ? "◌" : index + 1}</span>
          <strong>{label}</strong>
          <small>
            {index < active ? "Completed" : index === active ? "In progress" : "Waiting"}
          </small>
        </div>
      ))}
    </div>
  );
}

function joinWaitingTitle(status: SessionStatus) {
  if (status === "waiting_for_dns") return "Waiting for DNS to propagate.";
  if (isProvisioningLikeStatus(status)) return "Waiting for provisioning to start.";
  return "Waiting for your host.";
}

function JoinShell({ children }: { children: ReactNode }) {
  return (
    <div className="join">
      <main className="panel join-card">
        <Logo centered />
        {children}
      </main>
    </div>
  );
}

function StatusBadge({ status, label }: { status: SessionStatus; label?: string }) {
  return (
    <span className={`status ${sessionStatusClassName(status)} ${sessionStatusTone(status)}`}>
      <b>
        <Icon name={statusIcon(status)} />
      </b>
      {label ?? sessionStatusLabel(status)}
    </span>
  );
}

function Region({ region }: { region: string }) {
  return (
    <span className="region">
      {region.startsWith("eu") ? "🇪🇺" : region.startsWith("ap") ? "🇸🇬" : "🇺🇸"} {regionLabel(region)}
    </span>
  );
}

function regionLabel(region: string) {
  return (
    (
      {
        nyc3: "us-east-1",
        "us-east-1": "us-east-1",
        "us-west-2": "us-west-2",
        "eu-central-1": "eu-central-1",
        "eu-west-1": "eu-west-1",
        "ap-southeast-1": "ap-southeast-1",
      } as Record<string, string>
    )[region] ?? region
  );
}

function isAwaitingDownloadLikeStatus(status: SessionStatus): boolean {
  switch (status) {
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
      return true;
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "ready":
    case "active":
    case "tearing_down":
    case "ended":
    case "failed":
      return false;
    default:
      return exhaustiveStatus(status);
  }
}
function actionIcon(status: SessionStatus): IconName {
  switch (status) {
    case "active":
      return "square";
    case "finalizing":
    case "awaiting_manual_download":
    case "ended":
      return "download";
    case "teardown_pending":
    case "tearing_down":
    case "failed":
      return "refresh";
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "ready":
      return "play";
    default:
      return exhaustiveStatus(status);
  }
}
function statusIcon(status: SessionStatus): IconName {
  switch (status) {
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "finalizing":
      return "spinner";
    case "ready":
    case "ended":
      return "check";
    case "active":
      return "activity";
    case "awaiting_manual_download":
      return "download";
    case "teardown_pending":
      return "calendar";
    case "tearing_down":
      return "refresh";
    case "failed":
      return "triangle";
    default:
      return exhaustiveStatus(status);
  }
}
function exhaustiveStatus(status: never): never {
  throw new Error(`Unhandled session status: ${status}`);
}
function domainFor(session: Session) {
  return session.room_domain ?? `${session.slug}.cast.remote-tape.io`;
}

function Alert({ children }: { children: ReactNode }) {
  return <div className="panel error">{children}</div>;
}
function NotFound() {
  return (
    <Shell active="sessions">
      <Alert>Not found.</Alert>
    </Shell>
  );
}
function value(input: string | null) {
  return input && input.length > 0 ? input : "—";
}
function blankAsUndefined(input: string) {
  const trimmed = input.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}
function slugify(input: string) {
  return input
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 63);
}
function messageFromError(error: unknown) {
  return error instanceof Error ? error.message : "Request failed";
}
function isCreatedState(state: unknown): state is { created: CreateSessionResponse } {
  return Boolean(state && typeof state === "object" && "created" in state);
}
function formatDateTime(input: string) {
  return (
    <>
      <span>{formatDate(input)}</span>
      <br />
      <span>{formatTime(input)}</span>
    </>
  );
}
function formatDate(input: string) {
  const date = new Date(input);
  return Number.isNaN(date.getTime())
    ? input
    : new Intl.DateTimeFormat("en", { month: "short", day: "numeric", year: "numeric" }).format(
        date,
      );
}
function formatTime(input: string) {
  const date = new Date(input);
  return Number.isNaN(date.getTime())
    ? ""
    : new Intl.DateTimeFormat("en", { hour: "numeric", minute: "2-digit" }).format(date);
}
