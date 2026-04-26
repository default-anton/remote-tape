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
import { Logo, Shell } from "./Layout";
import { LoginPage } from "./LoginPage";
import { PlaceholderPage } from "./PlaceholderPage";
import type { CreateSessionResponse, Detail, Event, Session } from "./types";

const lifecycle = [
  "created",
  "provisioning",
  "waiting_for_dns",
  "ready",
  "active",
  "finalizing",
  "awaiting_manual_download",
  "teardown_pending",
  "ended",
];

const statusMeta: Record<string, { label: string; tone: string; icon: string }> = {
  created: { label: "created", tone: "orange", icon: "○" },
  provisioning: { label: "provisioning", tone: "orange", icon: "◌" },
  waiting_for_dns: { label: "waiting_for_dns", tone: "yellow", icon: "◷" },
  ready: { label: "ready", tone: "green", icon: "✓" },
  active: { label: "active", tone: "blue", icon: "⌁" },
  finalizing: { label: "finalizing", tone: "purple", icon: "◌" },
  awaiting_manual_download: { label: "awaiting_manual_download", tone: "purple", icon: "⇩" },
  teardown_pending: { label: "teardown_pending", tone: "yellow", icon: "↻" },
  tearing_down: { label: "tearing_down", tone: "yellow", icon: "↻" },
  ended: { label: "ended", tone: "gray", icon: "✓" },
  failed: { label: "failed", tone: "red", icon: "△" },
};

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
        if (["created", "provisioning", "waiting_for_dns"].includes(session.status))
          acc.provisioning += 1;
        if (session.status === "ready") acc.ready += 1;
        if (session.status === "active") acc.active += 1;
        if (session.status === "awaiting_manual_download") acc.awaiting += 1;
        if (session.status === "failed") acc.failed += 1;
        return acc;
      },
      { provisioning: 0, ready: 0, active: 0, awaiting: 0, failed: 0 },
    );
  }, [sessions]);

  return (
    <div className="stats mock-stats">
      <Stat
        tone="orange"
        icon="◌"
        label="Provisioning"
        value={counts.provisioning}
        hint="Creating resources"
      />
      <Stat tone="green" icon="✓" label="Ready" value={counts.ready} hint="Ready to start" />
      <Stat tone="blue" icon="⌁" label="Active" value={counts.active} hint="Currently recording" />
      <Stat
        tone="purple"
        icon="⇩"
        label="Awaiting download"
        value={counts.awaiting}
        hint="Recording complete"
      />
      <Stat tone="red" icon="△" label="Failed" value={counts.failed} hint="Needs attention" />
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
  icon: string;
  label: string;
  value: number;
  hint: string;
}) {
  return (
    <div className="stat cardish">
      <div className={`round-icon ${tone}`}>{icon}</div>
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
        <span>⌕</span>
        <input aria-label="Search sessions" placeholder="Search sessions…" />
        <kbd>⌘K</kbd>
      </label>
      <FakeSelect label="Status" value="All statuses" />
      <FakeSelect label="Region" value="All regions" />
      <FakeSelect label="Created" value="Any time" />
      <button type="button" className="button ghost">
        ▽ Filters
      </button>
      <button type="button" className="button icon-only" aria-label="Refresh">
        ↻
      </button>
    </div>
  );
}

function FakeSelect({ label, value }: { label: string; value: string }) {
  return (
    <button type="button" className="fake-select">
      <span>{label}</span>
      {value}
      <b>⌄</b>
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
          {sessions.map((session) => (
            <tr key={session.id}>
              <td>
                <span className={`tiny-dot ${statusTone(session.status)}`} />
                <Link
                  className="row-title"
                  to={{ pathname: `/sessions/${session.id}`, search: location.search }}
                >
                  {session.title}
                </Link>
                <br />
                <span className="row-id">{session.id}</span>
                <span className="copy-mini">□</span>
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
                  <button type="button">{actionIcon(session.status)}</button>
                  <button type="button">⋮</button>
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
          <button type="button">‹</button>
          <button className="current" type="button">
            1
          </button>
          <button type="button">2</button>
          <button type="button">3</button>
          <button type="button">›</button>
        </div>
        <button type="button">10 per page ⌄</button>
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
        <section className="panel form-panel">
          <CreateSessionForm
            busy={create.isPending}
            error={create.error}
            onSubmit={(input) => create.mutate(input)}
          />
        </section>
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
  const [region, setRegion] = useState("us-east-1");
  const [size, setSize] = useState("s-2vcpu-4gb");

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
        <input
          id="region"
          value={region}
          onChange={(event) => setRegion(event.target.value)}
          placeholder="Use backend default"
        />
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
          <button type="button">□</button>
        </div>
        <a href={`/join/${slug}`}>https://{slug || "session-slug"}.remote-tape.io/join</a>
      </Field>
      <Field label="Host name" help="Display name shown to guests in the session." count="12 / 80">
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
        Advanced options <span>›</span>
        <small>Tags, data retention, recording settings, and more.</small>
      </button>
      <div className="form-actions">
        <Link className="button ghost" to={{ pathname: "/sessions", search: location.search }}>
          Cancel
        </Link>
        <button className="primary" type="submit" disabled={busy}>
          {busy ? "Creating…" : "+ Create session"}
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
        {count ? <span>{count}</span> : null}
      </div>
      <p>{help}</p>
      {children}
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
            {tone === "do" ? "◖" : tone === "cf" ? "☁" : tone === "link" ? "∞" : "↻"}
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
            <code>{session.id}</code> <span className="copy-mini">□</span>{" "}
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
          {session.id} <span className="copy-mini">□</span>
        </Info>
        <Info label="Region">
          <Region region={session.droplet_region} />
        </Info>
        <Info label="Droplet IP">
          {value(session.droplet_ip)} <span className="copy-mini">□</span>
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

function Lifecycle({ status }: { status: string }) {
  const current = Math.max(0, lifecycle.indexOf(status));
  return (
    <section className="panel lifecycle">
      {lifecycle.map((step, index) => (
        <div
          className={`life-step ${index < current ? "done" : ""} ${index === current ? "current" : ""}`}
          key={step}
        >
          <span>{index < current ? "✓" : index === current ? statusMeta[step]?.icon : ""}</span>
          <strong>{step}</strong>
          <small>{index <= current ? `10:2${index}:0${index} AM` : ""}</small>
        </div>
      ))}
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

function JoinSteps({ status }: { status: string }) {
  const steps = [
    ["Creating droplet", "created"],
    ["Assigning IP", "provisioning"],
    ["Creating DNS", "waiting_for_dns"],
    ["Warming services", "ready"],
    ["Final health check", "active"],
  ];
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

function joinWaitingTitle(status: string) {
  return status === "waiting_for_dns"
    ? "Waiting for DNS to propagate."
    : "Waiting for provisioning to start.";
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

function StatusBadge({ status, label }: { status: string; label?: string }) {
  const meta = statusMeta[status] ?? { label: status, tone: "gray", icon: "○" };
  return (
    <span className={`status ${status.replaceAll("_", "-")} ${meta.tone}`}>
      <b>{meta.icon}</b>
      {label ?? meta.label}
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

function statusTone(status: string) {
  return statusMeta[status]?.tone ?? "gray";
}
function actionIcon(status: string) {
  return status === "active"
    ? "■"
    : status.includes("download") || status === "ended" || status === "finalizing"
      ? "⇩"
      : status.includes("teardown") || status === "failed"
        ? "↻"
        : "▷";
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
