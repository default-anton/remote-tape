import { HttpResponse, http, type HttpHandler } from "msw";
import { makeCreateSessionResponse, makeJoinResponse, makeSession } from "./fixtures";
import { detailForSession, scenario, validGuestToken, validHostToken } from "./scenarios";
import type { CreateSessionInput, JoinResponse, Session } from "../types";

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
      return HttpResponse.json({ sessions: sessionsFor(request) });
    }),
    http.get("/api/sessions/:id", ({ params, request }) => {
      const id = String(params.id ?? "");
      const session = sessionsFor(request).find((item) => item.id === id);
      if (!session) {
        return HttpResponse.json({ ok: false, error: "session not found" }, { status: 404 });
      }
      return HttpResponse.json(detailForSession(session));
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
      const session = makeSession({
        id: `sess_${slug.replaceAll("-", "_")}`,
        slug,
        title,
        droplet_region: input.droplet_region?.trim() || "nyc3",
        droplet_size: input.droplet_size?.trim() || "s-2vcpu-2gb",
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
      const role = roleForToken(slug, token, selectedScenario(request), created);
      const session = created?.session ?? sessionsFor(request).find((item) => item.slug === slug);

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
  const slug = title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 63);
  return slug || "untitled-session";
}
