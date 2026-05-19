import { HttpResponse, http, type HttpHandler } from "msw";
import {
  makeCreateSessionResponse,
  makeJoinResponse,
  makeProvisioningOptions,
  makeSession,
  makeSessionsResponse,
} from "./fixtures";
import { detailForSession, scenario, validGuestToken, validHostToken } from "./scenarios";
import type { CreateSessionInput, Event, JoinResponse, Session } from "../types";
import { slugify } from "../utils/forms";

type CreatedSession = {
  session: Session;
  hostToken: string;
  guestToken: string;
};

export type ControlMockApi = {
  handlers: HttpHandler[];
  reset: () => void;
};

export function createControlMockApi(): ControlMockApi {
  const createdSessions: CreatedSession[] = [];
  let authenticated: boolean | null = null;

  function sessionsFor(request: Request) {
    return [...createdSessions.map((item) => item.session), ...selectedScenario(request).sessions];
  }

  function createdSessionFor(slug: string) {
    return createdSessions.find((item) => item.session.slug === slug);
  }

  const handlers = [
    http.get("/api/auth/session", ({ request }) => {
      const isAuthenticated = authenticatedFor(request, authenticated);
      return HttpResponse.json({
        authenticated: isAuthenticated,
        subject: isAuthenticated ? "admin" : "",
        csrf_token: "test-csrf-token",
      });
    }),
    http.post("/login", () => {
      authenticated = true;
      return new HttpResponse(null, { status: 204 });
    }),
    http.post("/logout", () => {
      authenticated = false;
      return new HttpResponse(null, { status: 204 });
    }),
    http.get("/api/sessions", ({ request }) => {
      return HttpResponse.json(mockListSessionsResponse(request, sessionsFor(request)));
    }),
    http.get("/api/sessions/:id", ({ params, request }) => {
      const id = String(params.id ?? "");
      const session = sessionsFor(request).find((item) => item.id === id);
      if (!session) {
        return HttpResponse.json({ ok: false, error: "session not found" }, { status: 404 });
      }
      return HttpResponse.json(detailForSession(session));
    }),
    http.get("/api/sessions/:id/events", ({ params, request }) => {
      const id = String(params.id ?? "");
      const session = sessionsFor(request).find((item) => item.id === id);
      if (!session) {
        return HttpResponse.json({ ok: false, error: "session not found" }, { status: 404 });
      }
      return HttpResponse.json(
        mockSessionEventsResponse(request, detailForSession(session).events),
      );
    }),
    http.get("/api/session-slugs/:slug", ({ params, request }) => {
      const slug = String(params.slug ?? "")
        .trim()
        .toLowerCase();
      const valid = /^[a-z0-9-]{1,63}$/.test(slug) && !slug.startsWith("-") && !slug.endsWith("-");
      if (!valid) {
        return HttpResponse.json({
          slug: String(params.slug ?? ""),
          normalized_slug: slug,
          available: false,
          valid: false,
          reason: "invalid_format",
        });
      }
      const taken = sessionsFor(request).some((item) => item.slug === slug);
      return HttpResponse.json({
        slug: String(params.slug ?? ""),
        normalized_slug: slug,
        available: !taken,
        valid: true,
        reason: taken ? "taken" : null,
      });
    }),
    http.post("/api/sessions", async ({ request }) => {
      const input = (await request.json()) as Partial<CreateSessionInput>;
      const title = input.title?.trim() || "Untitled session";
      const slug = input.slug?.trim() || slugFromTitle(title);
      if (sessionsFor(request).some((item) => item.slug === slug)) {
        return HttpResponse.json(
          { ok: false, error: "session slug already exists" },
          { status: 409 },
        );
      }
      const provisioningOptions = makeProvisioningOptions();
      const session = makeSession({
        id: `sess_${slug.replaceAll("-", "_")}`,
        slug,
        title,
        instance_region: input.instance_region?.trim() || provisioningOptions.defaults.region,
        instance_size: input.instance_size?.trim() || provisioningOptions.defaults.size,
      });
      const created = {
        session,
        hostToken: `${slug}-host-token`,
        guestToken: `${slug}-guest-token`,
      };
      createdSessions.unshift(created);
      return HttpResponse.json(
        makeCreateSessionResponse({
          session,
          hostToken: created.hostToken,
          guestToken: created.guestToken,
          controlPlaneURL: new URL(request.url).origin,
        }),
        { status: 201 },
      );
    }),
    http.get("/api/join/:slug", ({ params, request }) => {
      const slug = String(params.slug ?? "");
      const url = new URL(request.url);
      const token = url.searchParams.get("token");
      const created = createdSessionFor(slug);
      const demo = demoJoinSession(slug, token);
      const role = demo?.role ?? roleForToken(slug, token, selectedScenario(request), created);
      const session =
        demo?.session ??
        created?.session ??
        sessionsFor(request).find((item) => item.slug === slug);

      if (!session || !role) {
        return HttpResponse.json({ ok: false, error: "join link not found" }, { status: 404 });
      }
      return HttpResponse.json(makeJoinResponse({ session, role }));
    }),
  ];

  return {
    handlers,
    reset() {
      createdSessions.length = 0;
      authenticated = null;
    },
  };
}

export function controlHandlers(): HttpHandler[] {
  return createControlMockApi().handlers;
}

function mockListSessionsResponse(request: Request, sessions: Session[]) {
  const url = new URL(request.url);
  const statuses = url.searchParams.getAll("status");
  const regions = url.searchParams.getAll("region");
  const query = (url.searchParams.get("q") ?? "").trim().toLowerCase();
  const sort = url.searchParams.get("sort") ?? "updated_at";
  const direction = url.searchParams.get("direction") === "asc" ? "asc" : "desc";
  let page = positiveInt(url.searchParams.get("page"), 1);
  const pageSize = positiveInt(url.searchParams.get("page_size"), 10);
  const filtered = sessions
    .filter((session) => statuses.length === 0 || statuses.includes(session.status))
    .filter((session) => regions.length === 0 || regions.includes(session.instance_region))
    .filter(
      (session) =>
        !query ||
        session.title.toLowerCase().includes(query) ||
        session.slug.toLowerCase().includes(query) ||
        session.id.toLowerCase().includes(query),
    )
    .sort((left, right) => compareSessions(left, right, sort, direction));
  const total = filtered.length;
  page = clampPage(page, pageSize, total);
  const start = (page - 1) * pageSize;
  const response = makeSessionsResponse(filtered);
  response.sessions = filtered.slice(start, start + pageSize);
  response.pagination = {
    page,
    page_size: pageSize,
    total,
    total_pages: total === 0 ? 0 : Math.ceil(total / pageSize),
  };
  return response;
}

function mockSessionEventsResponse(request: Request, events: Event[]) {
  const url = new URL(request.url);
  let page = positiveInt(url.searchParams.get("page"), 1);
  const pageSize = positiveInt(url.searchParams.get("page_size"), 10);
  const sorted = [...events].sort((left, right) => right.id - left.id);
  const total = sorted.length;
  page = clampPage(page, pageSize, total);
  const start = (page - 1) * pageSize;
  return {
    events: sorted.slice(start, start + pageSize),
    pagination: {
      page,
      page_size: pageSize,
      total,
      total_pages: total === 0 ? 0 : Math.ceil(total / pageSize),
    },
  };
}

function compareSessions(left: Session, right: Session, sort: string, direction: "asc" | "desc") {
  const multiplier = direction === "asc" ? 1 : -1;
  const leftValue = sessionSortValue(left, sort);
  const rightValue = sessionSortValue(right, sort);
  return multiplier * leftValue.localeCompare(rightValue);
}

function sessionSortValue(session: Session, sort: string) {
  switch (sort) {
    case "title":
      return session.title.toLowerCase();
    case "status":
      return session.status;
    case "region":
      return session.instance_region;
    case "room_domain":
      return session.room_domain ?? "";
    case "created_at":
      return session.created_at;
    default:
      return session.updated_at;
  }
}

function positiveInt(value: string | null, fallback: number) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function clampPage(page: number, pageSize: number, total: number) {
  if (total === 0) return page;
  return Math.min(page, Math.ceil(total / pageSize));
}

function demoJoinSession(slug: string, token: string | null) {
  if (slug !== "demo" || token !== "x") return null;
  return {
    role: "guest" as const,
    session: makeSession({ id: "sess_demo", slug: "demo", title: "Demo session" }),
  };
}

function roleForToken(
  slug: string,
  token: string | null,
  selected: ReturnType<typeof scenario>,
  created: CreatedSession | undefined,
): JoinResponse["token"]["role"] | null {
  if (created?.hostToken === token) return "host";
  if (created?.guestToken === token) return "guest";
  if (created) return null;
  if (slug !== selected.joinSlug) return null;
  if (token === validHostToken) return "host";
  if (token === validGuestToken) return "guest";
  return null;
}

function selectedScenario(request: Request) {
  const url = new URL(request.url);
  return scenario(url.searchParams.get("scenario") ?? browserScenarioName());
}

function browserScenarioName() {
  if (typeof window === "undefined") return null;
  return new URLSearchParams(window.location.search).get("scenario");
}

function authenticatedFor(request: Request, authenticated: boolean | null) {
  const mode = authMode(new URL(request.url)) ?? browserAuthMode();
  if (mode === "authenticated") return true;
  if (mode === "unauthenticated") return false;
  if (authenticated !== null) return authenticated;
  return !browserPathname().startsWith("/login");
}

function authMode(url: URL) {
  const value = url.searchParams.get("auth");
  return value === "authenticated" || value === "unauthenticated" ? value : null;
}

function browserAuthMode() {
  if (typeof window === "undefined") return null;
  return authMode(new URL(window.location.href));
}

function browserPathname() {
  if (typeof window === "undefined") return "/";
  return window.location.pathname;
}

function slugFromTitle(title: string) {
  return slugify(title);
}
