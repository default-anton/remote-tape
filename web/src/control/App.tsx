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
import type { CreateSessionResponse, Detail, Session } from "./types";

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/sessions" replace />} />
      <Route path="/sessions" element={<SessionsPage />} />
      <Route path="/sessions/:id" element={<SessionDetailPage />} />
      <Route path="/join/:slug" element={<JoinPage />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}

function SessionsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const sessions = useQuery({ queryKey: ["sessions"], queryFn: listSessions });
  const create = useMutation({
    mutationFn: createSession,
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({ queryKey: ["sessions"] });
      navigate(`/sessions/${created.session.id}`, { state: { created } });
    },
  });

  return (
    <Shell active="sessions">
      <div className="top">
        <div>
          <h1>Sessions</h1>
          <p className="muted">
            Create sessions, inspect lifecycle state, and keep the recording path observable.
          </p>
        </div>
        <a className="button primary" href="#create">
          + New session
        </a>
      </div>
      {sessions.isError ? <Alert>{messageFromError(sessions.error)}</Alert> : null}
      <Stats sessions={sessions.data?.sessions ?? []} />
      <div className="grid">
        <section className="card span-2">
          <h2>Session list</h2>
          {sessions.isLoading ? (
            <p className="muted">Loading sessions…</p>
          ) : (
            <SessionTable sessions={sessions.data?.sessions ?? []} />
          )}
        </section>
        <section id="create" className="card">
          <h2>Create session</h2>
          <p className="muted">
            Creation is fast. Provisioning will advance later through the reconciler.
          </p>
          <CreateSessionForm
            busy={create.isPending}
            error={create.error}
            onSubmit={(input) => create.mutate(input)}
          />
        </section>
      </div>
    </Shell>
  );
}

function Stats({ sessions }: { sessions: Session[] }) {
  const counts = useMemo(() => {
    return sessions.reduce(
      (acc, session) => {
        acc.total += 1;
        if (session.status === "ready") acc.ready += 1;
        if (session.status === "active") acc.active += 1;
        if (session.status === "failed") acc.failed += 1;
        return acc;
      },
      { total: 0, ready: 0, active: 0, failed: 0 },
    );
  }, [sessions]);

  return (
    <div className="stats">
      <Stat label="Total" value={counts.total} hint="Tracked sessions" />
      <Stat label="Ready" value={counts.ready} hint="Ready to join" />
      <Stat label="Active" value={counts.active} hint="Currently recording" />
      <Stat label="Failed" value={counts.failed} hint="Needs attention" />
    </div>
  );
}

function Stat({ label, value, hint }: { label: string; value: number; hint: string }) {
  return (
    <div className="stat">
      {label}
      <b>{value}</b>
      <span className="muted">{hint}</span>
    </div>
  );
}

function SessionTable({ sessions }: { sessions: Session[] }) {
  if (sessions.length === 0) {
    return <p className="muted">No sessions yet. Create one to get host and guest join links.</p>;
  }
  return (
    <table>
      <thead>
        <tr>
          <th>Session</th>
          <th>Status</th>
          <th>Region</th>
          <th>Room domain</th>
          <th>Updated</th>
        </tr>
      </thead>
      <tbody>
        {sessions.map((session) => (
          <tr key={session.id}>
            <td>
              <Link to={`/sessions/${session.id}`}>{session.title}</Link>
              <br />
              <span className="muted">{session.id}</span>
            </td>
            <td>
              <StatusBadge status={session.status} />
            </td>
            <td>{session.droplet_region}</td>
            <td>{value(session.room_domain)}</td>
            <td>{session.updated_at}</td>
          </tr>
        ))}
      </tbody>
    </table>
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
  const [title, setTitle] = useState("");
  const [slug, setSlug] = useState("");
  const [region, setRegion] = useState("");
  const [size, setSize] = useState("");

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
    <form onSubmit={submit}>
      {error ? <Alert>{messageFromError(error)}</Alert> : null}
      <label htmlFor="title">Session title</label>
      <input
        id="title"
        value={title}
        onChange={(event) => setTitle(event.target.value)}
        required
        maxLength={100}
        placeholder="The Infra Podcast #313"
      />
      <label htmlFor="slug">Session slug</label>
      <input
        id="slug"
        value={slug}
        onChange={(event) => setSlug(event.target.value)}
        pattern="[a-z0-9-]{1,63}"
        placeholder="the-infra-podcast-313"
      />
      <label htmlFor="region">Preferred region</label>
      <input
        id="region"
        value={region}
        onChange={(event) => setRegion(event.target.value)}
        placeholder="Use backend default"
      />
      <label htmlFor="size">Droplet size</label>
      <input
        id="size"
        value={size}
        onChange={(event) => setSize(event.target.value)}
        placeholder="Use backend default"
      />
      <button className="primary" type="submit" disabled={busy}>
        {busy ? "Creating…" : "+ Create session"}
      </button>
    </form>
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
      <p>
        <Link to="/sessions">← Back to sessions</Link>
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
      <div className="top">
        <div>
          <h1>{session.title}</h1>
          <p>
            <code>{session.id}</code> <StatusBadge status={session.status} />
          </p>
        </div>
      </div>
      {created ? <CreatedLinks created={created} /> : null}
      <section className="card space">
        <div className="kv">
          <span>Status</span>
          <span>
            <StatusBadge status={session.status} />
          </span>
          <span>Slug</span>
          <span>{session.slug}</span>
          <span>Region</span>
          <span>{session.droplet_region}</span>
          <span>Droplet ID</span>
          <span>{value(session.droplet_id)}</span>
          <span>Droplet IP</span>
          <span>{value(session.droplet_ip)}</span>
          <span>Room domain</span>
          <span>{value(session.room_domain)}</span>
          <span>DNS record ID</span>
          <span>{value(session.dns_record_id)}</span>
          <span>Last error</span>
          <span>{value(session.last_error)}</span>
        </div>
      </section>
      <section className="card space">
        <h2>Access tokens</h2>
        <div className="token-list">
          {detail.access_tokens.map((token) => (
            <div className="token-row" key={token.id}>
              <strong>{token.role}</strong>
              <span>{token.label ?? "—"}</span>
              <span>last used {value(token.last_used_at)}</span>
            </div>
          ))}
        </div>
      </section>
      <section className="card">
        <h2>Session events</h2>
        <ul className="timeline">
          {detail.events.map((event) => (
            <li key={event.id}>
              <span className="muted">{event.created_at}</span>
              <strong>{event.type}</strong>
              <span>{value(event.message)}</span>
            </li>
          ))}
        </ul>
      </section>
    </>
  );
}

function CreatedLinks({ created }: { created: CreateSessionResponse }) {
  return (
    <section className="card attention space">
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

  if (!slug || !token) {
    return (
      <JoinShell>
        <Alert>Join link is missing its token.</Alert>
      </JoinShell>
    );
  }
  if (join.isError) {
    return (
      <JoinShell>
        <Alert>{messageFromError(join.error)}</Alert>
      </JoinShell>
    );
  }
  if (join.isLoading) {
    return (
      <JoinShell>
        <p className="muted">Checking join link…</p>
      </JoinShell>
    );
  }
  if (!join.data) return null;

  return (
    <JoinShell>
      <h1>{join.data.session.title}</h1>
      <p>
        <StatusBadge status={join.data.session.status} label="Provisioning your room" />
      </p>
      <p className="muted">We'll redirect you automatically when the session is ready.</p>
      <div className="steps">
        <div className="step done">
          Creating droplet
          <br />
          <small>Queued</small>
        </div>
        <div className="step active">
          Assigning IP
          <br />
          <small>Waiting</small>
        </div>
        <div className="step">
          Creating DNS
          <br />
          <small>Waiting</small>
        </div>
        <div className="step">
          Warming services
          <br />
          <small>Waiting</small>
        </div>
        <div className="step">
          Final health check
          <br />
          <small>Waiting</small>
        </div>
      </div>
      <h2>Waiting for provisioning to start.</h2>
      <p className="muted">Polling and redirect behavior lands with the room redirect slice.</p>
      <p>
        <strong>Session</strong>
        <br />
        <code>{join.data.session.slug}</code>
      </p>
      <p>
        <strong>You're joining as</strong>
        <br />
        {join.data.token.role}
      </p>
    </JoinShell>
  );
}

function JoinShell({ children }: { children: ReactNode }) {
  return (
    <div className="join">
      <main className="card join-card">
        <div className="logo">remote-tape</div>
        <div className="tagline">remote podcast recorder</div>
        {children}
      </main>
    </div>
  );
}

function Shell({ active, children }: { active: "sessions"; children: ReactNode }) {
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="logo">remote-tape</div>
        <div className="tagline">remote podcast recorder</div>
        <nav className="nav">
          <Link className={active === "sessions" ? "active" : ""} to="/sessions">
            Sessions
          </Link>
          <a href="/readyz">Diagnostics</a>
        </nav>
        <div className="health">
          <span className="dot" />
          Control plane
          <br />
          <span className="muted">SQLite-backed local state</span>
        </div>
      </aside>
      <main className="main">{children}</main>
    </div>
  );
}

function StatusBadge({ status, label }: { status: string; label?: string }) {
  return <span className={`status ${status.replaceAll("_", "-")}`}>{label ?? status}</span>;
}

function Alert({ children }: { children: ReactNode }) {
  return <div className="card error">{children}</div>;
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

function messageFromError(error: unknown) {
  return error instanceof Error ? error.message : "Request failed";
}

function isCreatedState(state: unknown): state is { created: CreateSessionResponse } {
  return Boolean(state && typeof state === "object" && "created" in state);
}
