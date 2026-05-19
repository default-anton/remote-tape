import type {
  AccessToken,
  CreateSessionResponse,
  Detail,
  Event,
  JoinResponse,
  ProvisioningOptions,
  Session,
  SessionsResponse,
} from "../types";

const timestamp = "2026-04-25T10:00:00.000000000Z";
const defaultControlPlaneURL = "http://127.0.0.1:5173";

export function makeProvisioningOptions(
  overrides: Partial<ProvisioningOptions> = {},
): ProvisioningOptions {
  return {
    defaults: { region: "nyc3", size: "s-2vcpu-4gb" },
    regions: [
      { slug: "nyc3", label: "New York 3" },
      { slug: "sfo2", label: "San Francisco 2" },
    ],
    sizes: [
      {
        slug: "s-1vcpu-512mb-10gb",
        label: "Shared CPU Basic",
        description:
          "1 vCPU / 512 MB / 10 GB — development-only cheapest size; not for recording-quality validation",
        recommended: false,
        dedicated_cpu: false,
      },
      {
        slug: "s-2vcpu-2gb",
        label: "Shared CPU Basic",
        description: "2 vCPU / 2 GB / 60 GB — low-cost small session",
        recommended: false,
        dedicated_cpu: false,
      },
      {
        slug: "s-2vcpu-4gb",
        label: "Shared CPU Basic",
        description: "2 vCPU / 4 GB / 80 GB — recommended default",
        recommended: true,
        dedicated_cpu: false,
      },
      {
        slug: "c-2",
        label: "Dedicated CPU CPU-Optimized",
        description: "2 vCPU / 4 GB / 25 GB — recommended production session",
        recommended: true,
        dedicated_cpu: true,
      },
    ],
    availability: {
      nyc3: ["s-1vcpu-512mb-10gb", "s-2vcpu-2gb", "s-2vcpu-4gb"],
      sfo2: ["s-1vcpu-512mb-10gb", "s-2vcpu-2gb", "s-2vcpu-4gb", "c-2"],
    },
    recommended_size_by_region: {
      nyc3: "s-2vcpu-4gb",
      sfo2: "s-2vcpu-4gb",
    },
    ...overrides,
  };
}

export function makeSessionsResponse(sessions: Session[]): SessionsResponse {
  return {
    sessions,
    pagination: {
      page: 1,
      page_size: 10,
      total: sessions.length,
      total_pages: sessions.length === 0 ? 0 : Math.ceil(sessions.length / 10),
    },
    summary: sessionSummary(sessions),
    filters: {
      statuses: [
        { value: "created", label: "Created" },
        { value: "provisioning", label: "Provisioning" },
        { value: "waiting_for_dns", label: "Waiting for DNS" },
        { value: "ready", label: "Ready" },
        { value: "active", label: "Active" },
        { value: "finalizing", label: "Finalizing" },
        { value: "awaiting_manual_download", label: "Awaiting manual download" },
        { value: "teardown_pending", label: "Teardown pending" },
        { value: "tearing_down", label: "Tearing down" },
        { value: "ended", label: "Ended" },
        { value: "failed", label: "Failed" },
      ],
      regions: makeProvisioningOptions().regions.map((region) => ({
        value: region.slug,
        label: region.label,
      })),
    },
    has_pollable: sessions.some((session) => pollableSessionStatus(session.status)),
    provisioning_options: makeProvisioningOptions(),
  };
}

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
    instance_size: "s-2vcpu-4gb",
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

function sessionSummary(sessions: Session[]): SessionsResponse["summary"] {
  return {
    total: sessions.length,
    provisioning: sessions.filter((session) =>
      ["created", "provisioning", "waiting_for_dns"].includes(session.status),
    ).length,
    ready: sessions.filter((session) => session.status === "ready").length,
    active: sessions.filter((session) => session.status === "active").length,
    awaiting_manual_download: sessions.filter((session) =>
      ["finalizing", "awaiting_manual_download", "teardown_pending"].includes(session.status),
    ).length,
    failed: sessions.filter((session) => session.status === "failed").length,
  };
}

function pollableSessionStatus(status: Session["status"]) {
  return !["ended", "failed"].includes(status);
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
