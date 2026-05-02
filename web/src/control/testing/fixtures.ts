import type {
  AccessToken,
  CreateSessionResponse,
  Detail,
  Event,
  JoinResponse,
  Session,
} from "../types";

const timestamp = "2026-04-25T10:00:00.000000000Z";
const defaultControlPlaneURL = "http://127.0.0.1:5173";

export function makeSession(overrides: Partial<Session> = {}): Session {
  const id = overrides.id ?? `sess_${overrides.status ?? "created"}`;
  const slug = overrides.slug ?? id.replace(/^sess_/, "").replaceAll("_", "-");
  const title = overrides.title ?? titleFromSlug(slug);

  return {
    id,
    slug,
    title,
    status: "created",
    instance_id: null,
    public_ip: null,
    instance_region: "nyc3",
    instance_size: "s-2vcpu-2gb",
    image_id: "ubuntu-24-04-x64",
    room_domain: mockRoomDomain(id),
    dns_record_id: null,
    livekit_url: null,
    recording_download_url: null,
    finalization_summary_json: null,
    created_at: timestamp,
    updated_at: timestamp,
    ready_at: null,
    active_at: null,
    finalization_started_at: null,
    finalized_at: null,
    last_heartbeat_at: null,
    download_confirmed_at: null,
    download_confirmed_by: null,
    ended_at: null,
    expires_at: null,
    last_error: null,
    last_error_at: null,
    last_error_phase: null,
    provision_attempts: 0,
    dns_attempts: 0,
    health_attempts: 0,
    teardown_attempts: 0,
    ...overrides,
  };
}

export function makeAccessToken(
  session: Session,
  role: AccessToken["role"],
  overrides: Partial<AccessToken> = {},
): AccessToken {
  return {
    id: `sat_${session.slug}_${role}`,
    session_id: session.id,
    role,
    label: `Initial ${role}`,
    created_at: timestamp,
    last_used_at: null,
    revoked_at: null,
    ...overrides,
  };
}

export function makeEvent(session: Session, overrides: Partial<Event> = {}): Event {
  return {
    id: 1,
    session_id: session.id,
    type: "session.created",
    message: null,
    metadata_json: null,
    created_at: timestamp,
    ...overrides,
  };
}

export function makeDetail({
  session = makeSession(),
  access_tokens,
  events,
}: {
  session?: Session;
  access_tokens?: AccessToken[];
  events?: Event[];
} = {}): Detail {
  return {
    session,
    access_tokens: access_tokens ?? [
      makeAccessToken(session, "host"),
      makeAccessToken(session, "guest"),
    ],
    events: events ?? [makeEvent(session, { type: "session.created", message: "Session created" })],
  };
}

export function makeCreateSessionResponse({
  session = makeSession(),
  hostToken = "host-token",
  guestToken = "guest-token",
  controlPlaneURL = defaultControlPlaneURL,
}: {
  session?: Session;
  hostToken?: string;
  guestToken?: string;
  controlPlaneURL?: string;
} = {}): CreateSessionResponse {
  return {
    session,
    join_links: {
      host: { url: joinURL(controlPlaneURL, session.slug, hostToken), role: "host" },
      guest: { url: joinURL(controlPlaneURL, session.slug, guestToken), role: "guest" },
    },
    events: [makeEvent(session, { type: "session.created", message: "Session created" })],
    tokens: {
      host: { id: `sat_${session.slug}_host`, token: hostToken },
      guest: { id: `sat_${session.slug}_guest`, token: guestToken },
    },
  };
}

export function makeJoinResponse({
  session = makeSession({ slug: "joinable", title: "Joinable" }),
  role = "guest",
}: {
  session?: Session;
  role?: JoinResponse["token"]["role"];
} = {}): JoinResponse {
  return {
    session: {
      slug: session.slug,
      title: session.title,
      status: session.status,
    },
    token: { role },
  };
}

function titleFromSlug(slug: string) {
  return slug
    .split("-")
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
}

function mockRoomDomain(seed: string) {
  return `room-${mockBase32(seed, 26)}.sessions.localhost`;
}

function mockBase32(seed: string, length: number) {
  const alphabet = "abcdefghijklmnopqrstuvwxyz234567";
  let output = "";
  let counter = 0;
  while (output.length < length) {
    let hash = 2166136261;
    for (const char of `${seed}:${counter}`) {
      hash ^= char.charCodeAt(0);
      hash = Math.imul(hash, 16777619) >>> 0;
    }
    for (let i = 0; i < 6 && output.length < length; i += 1) {
      output += alphabet[hash & 31];
      hash >>>= 5;
    }
    counter += 1;
  }
  return output;
}

function joinURL(controlPlaneURL: string, slug: string, token: string) {
  const base = controlPlaneURL.replace(/\/+$/, "");
  return `${base}/join/${encodeURIComponent(slug)}?token=${encodeURIComponent(token)}`;
}
