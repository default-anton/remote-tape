import { type ReactNode } from "react";
import { useParams, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { joinSession } from "../api";
import { Alert } from "../components/Alert";
import { Logo } from "../components/Shell";
import { StatusBadge } from "../components/StatusBadge";
import { isProvisioningLikeStatus, type SessionStatus } from "../domain/sessionStatus";
import { messageFromError } from "../utils/errors";

export function JoinPage() {
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
