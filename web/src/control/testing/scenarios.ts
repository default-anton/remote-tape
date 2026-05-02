import { makeDetail, makeSession } from "./fixtures";
import { SESSION_STATUSES, type SessionStatus } from "../domain/sessionStatus";
import type { Detail, Session } from "../types";

export const lifecycleStatuses = SESSION_STATUSES;

export type ScenarioName = "mixed" | "joinable" | "provisioning_failed" | SessionStatus;

export type ControlScenario = {
  name: ScenarioName;
  sessions: Session[];
  joinSlug: string;
};

export const validGuestToken = "guest-token";
export const validHostToken = "host-token";

const statusDetails: Partial<Record<SessionStatus, Partial<Session>>> = {
  provisioning: {
    provision_attempts: 1,
  },
  waiting_for_dns: {
    instance_id: "123456789",
    public_ip: "203.0.113.10",
    dns_attempts: 1,
  },
  ready: {
    instance_id: "123456789",
    public_ip: "203.0.113.10",
    dns_record_id: "dns_ready",
    livekit_url: "wss://room-ready.sessions.localhost/livekit",
    ready_at: "2026-04-25T10:02:00.000000000Z",
    health_attempts: 1,
    // TODO: add room redirect target when the production join API exposes it.
  },
  active: {
    instance_id: "123456789",
    public_ip: "203.0.113.10",
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
    last_error: "instance health check failed",
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
  if (normalized === "provisioning_failed") return provisioningFailedScenario();
  return statusScenario(normalized);
}

export function detailForSession(session: Session): Detail {
  return makeDetail({
    session,
    events: eventsForSession(session),
  });
}

function mixedScenario(): ControlScenario {
  const rows: Array<Partial<Session> & { status: SessionStatus }> = [
    {
      id: "sess_01HZXJ7K9Q13XW9YA4B87C5",
      slug: "joinable",
      title: "The Infra Podcast #312",
      status: "provisioning",
      instance_region: "us-east-1",
      room_domain: "theinfra.cast.remote-tape.io",
    },
    {
      id: "sess_01HZX6F8T2P5Q7R1V0D96F3",
      slug: "product-builders-live",
      title: "Product Builders Live",
      status: "waiting_for_dns",
      instance_region: "us-west-2",
      room_domain: "pblive.cast.remote-tape.io",
    },
    {
      id: "sess_01HZX4M1B6N8D2C3V7E5F9A1",
      slug: "syntax-fm-recording",
      title: "Syntax.fm – Recording",
      status: "ready",
      instance_region: "us-east-1",
      room_domain: "syntax.cast.remote-tape.io",
    },
    {
      id: "sess_01HZX2Y9Y3K6L8M0N4Q1W2E7",
      slug: "founders-unplugged",
      title: "Founders Unplugged",
      status: "active",
      instance_region: "eu-central-1",
      room_domain: "founders.cast.remote-tape.io",
    },
    {
      id: "sess_01HZWZ8J1R4T6V3B9N0C2D5E",
      slug: "devrel-show",
      title: "The DevRel Show",
      status: "finalizing",
      instance_region: "us-west-2",
      room_domain: "devrel.cast.remote-tape.io",
    },
    {
      id: "sess_01HZWX0Y6P2D4F8G1H7J9K3L",
      slug: "ai-and-coffee",
      title: "AI & Coffee",
      status: "awaiting_manual_download",
      instance_region: "us-east-1",
      room_domain: "aiandcoffee.cast.remote-tape.io",
    },
    {
      id: "sess_01HZWV7E0M5C9B2N6R3T8Y1U",
      slug: "open-source-today",
      title: "Open Source Today",
      status: "teardown_pending",
      instance_region: "eu-west-1",
      room_domain: "oss.cast.remote-tape.io",
    },
    {
      id: "sess_01HZWQ3PBV4K2M6N9D1E5F7G",
      slug: "latent-space",
      title: "Latent Space",
      status: "ended",
      instance_region: "us-west-2",
      room_domain: "latent.cast.remote-tape.io",
    },
    {
      id: "sess_01HZWJ2H9L6B7V3C1X4Z8A5S",
      slug: "marketing-trends-podcast",
      title: "Marketing Trends Podcast",
      status: "failed",
      instance_region: "ap-southeast-1",
      room_domain: "mktgtrends.cast.remote-tape.io",
    },
    {
      id: "sess_01HZW8D6K3T9R1V4B7N5M2QW",
      slug: "no-priors",
      title: "No Priors",
      status: "ended",
      instance_region: "us-east-1",
      room_domain: "nopriors.cast.remote-tape.io",
    },
  ];
  const extraRows: Array<Partial<Session> & { status: SessionStatus }> = [
    { title: "Provisioning Backfill", status: "created" },
    ...Array.from({ length: 7 }, (_, index) => ({
      title: `Ready Session ${index + 1}`,
      status: "ready" as const,
    })),
    ...Array.from({ length: 4 }, (_, index) => ({
      title: `Active Session ${index + 1}`,
      status: "active" as const,
    })),
    { title: "Download Backfill", status: "awaiting_manual_download" },
    { title: "Failed Backfill", status: "failed" },
  ];
  const createdAt = [
    "2025-05-17T17:21:00.000000000Z",
    "2025-05-17T16:58:00.000000000Z",
    "2025-05-17T16:30:00.000000000Z",
    "2025-05-17T16:00:00.000000000Z",
    "2025-05-17T15:15:00.000000000Z",
    "2025-05-17T06:47:00.000000000Z",
    "2025-05-17T05:05:00.000000000Z",
    "2025-05-16T14:30:00.000000000Z",
    "2025-05-17T01:45:00.000000000Z",
    "2025-05-17T00:20:00.000000000Z",
  ];
  const updatedAt = [
    "2025-05-17T17:22:00.000000000Z",
    "2025-05-17T17:01:00.000000000Z",
    "2025-05-17T16:31:00.000000000Z",
    "2025-05-17T16:42:00.000000000Z",
    "2025-05-17T16:10:00.000000000Z",
    "2025-05-17T15:44:00.000000000Z",
    "2025-05-17T14:20:00.000000000Z",
    "2025-05-16T16:02:00.000000000Z",
    "2025-05-17T01:48:00.000000000Z",
    "2025-05-17T01:15:00.000000000Z",
  ];
  const sessions = [...rows, ...extraRows].map((row, index) =>
    makeSession({
      id: row.id ?? `sess_01HZWMIXED${String(index).padStart(2, "0")}`,
      slug: row.slug ?? `mixed-session-${index}`,
      room_domain: row.room_domain ?? `session-${index}.cast.remote-tape.io`,
      created_at: createdAt[index] ?? "2025-05-16T17:00:00.000000000Z",
      updated_at: updatedAt[index] ?? "2025-05-17T17:00:00.000000000Z",
      ...statusDetails[row.status],
      ...row,
    }),
  );
  return { name: "mixed", sessions, joinSlug: "joinable" };
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
    title: "The Infra Podcast #312",
  });
  return { name: status, sessions: [session], joinSlug: session.slug };
}

function provisioningFailedScenario(): ControlScenario {
  const session = makeSession({
    id: "sess_joinable_provisioning_failed",
    slug: "joinable",
    title: "The Infra Podcast #312",
    status: "failed",
    provision_attempts: 1,
    last_error: "DigitalOcean create instance request failed: rate limited",
    last_error_at: "2026-04-25T10:03:00.000000000Z",
    last_error_phase: "provisioning",
  });
  return { name: "provisioning_failed", sessions: [session], joinSlug: session.slug };
}

function eventsForSession(session: Session) {
  const events = [
    {
      id: 1,
      session_id: session.id,
      type: "session.created",
      message: "Session created",
      metadata_json: null,
      created_at: session.created_at,
    },
  ];
  if (session.provision_attempts > 0) {
    events.push({
      id: events.length + 1,
      session_id: session.id,
      type: "provisioning.started",
      message: "Provisioning started",
      metadata_json: null,
      created_at: session.updated_at,
    });
  }
  if (session.status === "failed" && session.last_error_phase === "provisioning") {
    events.push({
      id: events.length + 1,
      session_id: session.id,
      type: "provisioning.failed",
      message: "Provisioning failed",
      metadata_json: null,
      created_at: session.last_error_at ?? session.updated_at,
    });
  } else if (session.status !== "created" && session.status !== "provisioning") {
    events.push({
      id: events.length + 1,
      session_id: session.id,
      type: `session.${session.status}`,
      message: `Fixture state: ${session.status}`,
      metadata_json: null,
      created_at: session.updated_at,
    });
  }
  return events;
}

function normalizeScenarioName(name: string | null | undefined): ScenarioName {
  if (!name) return "mixed";
  if (name === "mixed" || name === "joinable" || name === "provisioning_failed") return name;
  if (lifecycleStatuses.includes(name as SessionStatus)) return name as SessionStatus;
  return "mixed";
}

function titleForStatus(status: SessionStatus) {
  return status
    .split("_")
    .map((word) => word[0]?.toUpperCase() + word.slice(1))
    .join(" ");
}
