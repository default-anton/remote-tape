import { makeDetail, makeSession, type SessionStatus } from "./fixtures";
import type { Detail, Session } from "../types";

export const lifecycleStatuses = [
  "created",
  "provisioning",
  "waiting_for_dns",
  "ready",
  "active",
  "finalizing",
  "awaiting_manual_download",
  "teardown_pending",
  "tearing_down",
  "ended",
  "failed",
] as const satisfies readonly SessionStatus[];

export type ScenarioName = "mixed" | "joinable" | "provisioning" | SessionStatus;

export type ControlScenario = {
  name: ScenarioName;
  sessions: Session[];
  joinSlug: string;
};

export const validGuestToken = "guest-token";
export const validHostToken = "host-token";

const statusDetails: Partial<Record<SessionStatus, Partial<Session>>> = {
  provisioning: {
    droplet_id: "123456789",
    provision_attempts: 1,
  },
  waiting_for_dns: {
    droplet_id: "123456789",
    droplet_ip: "203.0.113.10",
    dns_attempts: 1,
  },
  ready: {
    droplet_id: "123456789",
    droplet_ip: "203.0.113.10",
    dns_record_id: "dns_ready",
    livekit_url: "wss://room-ready.sessions.localhost/livekit",
    ready_at: "2026-04-25T10:02:00.000000000Z",
    health_attempts: 1,
    // TODO: add room redirect target when the production join API exposes it.
  },
  active: {
    droplet_id: "123456789",
    droplet_ip: "203.0.113.10",
    dns_record_id: "dns_active",
    livekit_url: "wss://room-active.sessions.localhost/livekit",
    ready_at: "2026-04-25T10:02:00.000000000Z",
    active_at: "2026-04-25T10:04:00.000000000Z",
    last_heartbeat_at: "2026-04-25T10:05:00.000000000Z",
  },
  finalizing: {
    finalization_started_at: "2026-04-25T10:30:00.000000000Z",
  },
  awaiting_manual_download: {
    finalization_started_at: "2026-04-25T10:30:00.000000000Z",
    finalized_at: "2026-04-25T10:35:00.000000000Z",
    recording_download_url:
      "https://room-awaiting-manual-download.sessions.localhost/downloads/session.zip",
    finalization_summary_json: '{"chunks":12,"warnings":0}',
  },
  teardown_pending: {
    download_confirmed_at: "2026-04-25T10:40:00.000000000Z",
    download_confirmed_by: "host@example.test",
  },
  tearing_down: {
    teardown_attempts: 1,
  },
  ended: {
    ended_at: "2026-04-25T10:45:00.000000000Z",
  },
  failed: {
    last_error: "droplet health check failed",
    last_error_at: "2026-04-25T10:03:00.000000000Z",
    last_error_phase: "health_check",
    health_attempts: 3,
  },
};

export function makeLifecycleSession(status: SessionStatus): Session {
  const slug = status.replaceAll("_", "-");
  return makeSession({
    id: `sess_${status}`,
    slug,
    title: titleForStatus(status),
    status,
    ...statusDetails[status],
  });
}

export function scenario(name: string | null | undefined): ControlScenario {
  const normalized = normalizeScenarioName(name);
  if (normalized === "mixed") return mixedScenario();
  if (normalized === "joinable") return joinableScenario();
  if (normalized === "provisioning") return statusScenario("provisioning");
  return statusScenario(normalized);
}

export function detailForSession(session: Session): Detail {
  return makeDetail({
    session,
    events: [
      {
        id: 1,
        session_id: session.id,
        type: "session.created",
        message: "Session created",
        metadata_json: null,
        created_at: session.created_at,
      },
      {
        id: 2,
        session_id: session.id,
        type: `session.${session.status}`,
        message: `Fixture state: ${session.status}`,
        metadata_json: null,
        created_at: session.updated_at,
      },
    ],
  });
}

function mixedScenario(): ControlScenario {
  const joinable = makeSession({ id: "sess_joinable", slug: "joinable", title: "Joinable" });
  return {
    name: "mixed",
    sessions: [joinable, ...lifecycleStatuses.map(makeLifecycleSession)],
    joinSlug: joinable.slug,
  };
}

function joinableScenario(): ControlScenario {
  const session = makeSession({ id: "sess_joinable", slug: "joinable", title: "Joinable" });
  return { name: "joinable", sessions: [session], joinSlug: session.slug };
}

function statusScenario(status: SessionStatus): ControlScenario {
  const session = makeSession({
    ...makeLifecycleSession(status),
    id: `sess_joinable_${status}`,
    slug: "joinable",
    title: `Joinable ${titleForStatus(status)}`,
  });
  return { name: status, sessions: [session], joinSlug: session.slug };
}

function normalizeScenarioName(name: string | null | undefined): ScenarioName {
  if (!name) return "mixed";
  if (name === "mixed" || name === "joinable" || name === "provisioning") return name;
  if (lifecycleStatuses.includes(name as SessionStatus)) return name as SessionStatus;
  return "mixed";
}

function titleForStatus(status: SessionStatus) {
  return status
    .split("_")
    .map((word) => word[0]?.toUpperCase() + word.slice(1))
    .join(" ");
}
