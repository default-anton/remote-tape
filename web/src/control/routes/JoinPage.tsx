import { type ReactNode } from "react";
import { useParams, useSearchParams } from "react-router";
import { useJoinSession } from "../api/hooks";
import { Alert } from "../components/Alert";
import { Logo } from "../components/Shell";
import { StatusBadge } from "../components/StatusBadge";
import {
  isJoinRedirectReadyStatus,
  isProvisioningLikeStatus,
  isTerminalStatus,
  shouldPollJoin,
  type SessionStatus,
} from "../domain/sessionStatus";
import { messageFromError } from "../utils/errors";

export function JoinPage() {
  const { slug } = useParams();
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const join = useJoinSession(slug, token);

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

  const status = join.data.session.status;
  const state = joinState(status);

  return (
    <JoinShell>
      <h1>{join.data.session.title}</h1>
      <p>
        <StatusBadge status={status} label={state.badgeLabel} />
      </p>
      <p className="join-lead">{state.lead}</p>
      {state.showSteps ? <JoinSteps status={status} /> : null}
      {state.kind === "waiting" ? <WaitingPanel status={status} /> : null}
      {state.kind === "ready" ? <ReadyPanel /> : null}
      {state.kind === "unavailable" ? <UnavailablePanel status={status} /> : null}
      <JoinMeta slug={join.data.session.slug} role={join.data.token.role} />
      <p className="info-note">
        ⓘ If this takes unusually long, <a href="mailto:host@example.test">contact your host</a>.
      </p>
    </JoinShell>
  );
}

function joinState(status: SessionStatus) {
  if (shouldPollJoin(status)) {
    return {
      kind: "waiting",
      badgeLabel: "Provisioning your room",
      lead: "We’ll keep checking until the session is ready.",
      showSteps: true,
    } as const;
  }
  if (isJoinRedirectReadyStatus(status)) {
    return {
      kind: "ready",
      badgeLabel: "Room is ready",
      lead: "The session is ready to join.",
      showSteps: true,
    } as const;
  }
  return {
    kind: "unavailable",
    badgeLabel: isTerminalStatus(status) ? "Session unavailable" : "Join unavailable",
    lead: "This session is not accepting new joins right now.",
    showSteps: false,
  } as const;
}

function WaitingPanel({ status }: { status: SessionStatus }) {
  return (
    <div className="waiting">
      <div className="spinner" />
      <h2>{joinWaitingTitle(status)}</h2>
      <p className="muted">This usually takes 10–30 seconds.</p>
      <p>
        <span className="live-dot" /> Polling every 5 seconds
      </p>
    </div>
  );
}

function ReadyPanel() {
  return (
    <div className="waiting">
      <h2>Room is ready.</h2>
      <p className="muted">
        If you are not redirected automatically, contact the host for the room URL.
      </p>
    </div>
  );
}

function UnavailablePanel({ status }: { status: SessionStatus }) {
  return (
    <Alert>
      {status === "failed"
        ? "This session failed before it became joinable. Contact the host for a new link."
        : status === "ended"
          ? "This session has ended and can no longer be joined."
          : "This session is past the joinable phase."}
    </Alert>
  );
}

function JoinMeta({ slug, role }: { slug: string; role: string }) {
  return (
    <div className="join-meta">
      <div>
        <span className="round-icon gray">↗</span>
        <p>
          <small>Session</small>
          <strong>{slug}</strong>
        </p>
      </div>
      <div>
        <span className="round-icon gray">♙</span>
        <p>
          <small>You're joining as</small>
          <strong>{role}</strong>
        </p>
      </div>
    </div>
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
