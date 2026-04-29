import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import {
  joinRefetchInterval,
  sessionDetailRefetchInterval,
  sessionsListRefetchInterval,
} from "./api/hooks";
import { isAttentionStatus } from "./domain/sessionStatus";
import {
  makeAccessToken,
  makeDetail,
  makeEvent,
  makeJoinResponse,
  makeSession,
} from "./testing/fixtures";
import { createControlMockApi } from "./testing/handlers";
import { renderApp } from "./testing/renderApp";
import { SessionSchema } from "./types";

const mockApi = createControlMockApi();
const server = setupServer(...mockApi.handlers);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  mockApi.reset();
  window.history.replaceState({}, "", "/");
});
afterAll(() => server.close());

describe("control app", () => {
  it("renders sessions from the API", async () => {
    renderApp("/sessions?scenario=joinable");

    expect(await screen.findByRole("link", { name: "Joinable" })).toBeInTheDocument();
    expect(screen.getByText(/^room-[a-z2-7]{26}\.sessions\.localhost$/)).toBeInTheDocument();
  });

  it("renders multiple lifecycle statuses in the sessions list", async () => {
    renderApp("/sessions?scenario=mixed");

    expect(await screen.findByText("provisioning")).toBeInTheDocument();
    expect(screen.getByText("waiting_for_dns")).toBeInTheDocument();
    expect(screen.getByText("ready")).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
    expect(screen.getByText("finalizing")).toBeInTheDocument();
    expect(screen.getByText("awaiting_manual_download")).toBeInTheDocument();
    expect(screen.getByText("teardown_pending")).toBeInTheDocument();
    expect(screen.getAllByText("ended").length).toBeGreaterThan(0);
    expect(screen.getByText("failed")).toBeInTheDocument();
  });

  it("shows dashboard stats for total, ready, active, and failed sessions", async () => {
    renderApp("/sessions?scenario=mixed");

    await waitFor(() => expect(statValue("Total sessions")).toBe("24"));
    expect(statValue("Ready sessions")).toBe("8");
    expect(statValue("Active sessions")).toBe("5");
    expect(statValue("Failed sessions")).toBe("2");
  });

  it("renders an empty sessions list with a creation prompt", async () => {
    server.use(http.get("/api/sessions", () => HttpResponse.json({ sessions: [] })));

    renderApp("/sessions");

    expect(
      await screen.findByText("No sessions yet. Create one to get host and guest join links."),
    ).toBeInTheDocument();
  });

  it("keeps mock login unauthenticated until POST /login", async () => {
    renderApp("/login");

    expect(await screen.findByLabelText("Password")).toBeInTheDocument();
    await expect(fetchAuthSession()).resolves.toMatchObject({ authenticated: false });

    await fetch("/login", { method: "POST" });
    await expect(fetchAuthSession()).resolves.toMatchObject({
      authenticated: true,
      subject: "admin",
    });

    await fetch("/logout", { method: "POST" });
    await expect(fetchAuthSession()).resolves.toMatchObject({ authenticated: false });
  });

  it("submits the React login form with CSRF and renders failures", async () => {
    let csrfHeader: string | null = null;
    let postedPassword: string | null = null;
    server.use(
      http.get("/api/auth/session", () =>
        HttpResponse.json({ authenticated: false, subject: "", csrf_token: "login-csrf" }),
      ),
      http.post("/login", async ({ request }) => {
        csrfHeader = request.headers.get("X-CSRF-Token");
        postedPassword = (await request.formData()).get("password")?.toString() ?? null;
        return HttpResponse.json({ ok: false, error: "invalid password" }, { status: 401 });
      }),
    );

    renderApp("/login");

    fireEvent.change(await screen.findByLabelText("Password"), {
      target: { value: "dev-password" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Sign in →" }).closest("form")!);

    await waitFor(() => expect(postedPassword).toBe("dev-password"));
    expect(csrfHeader).toBe("login-csrf");
    expect(await screen.findByRole("alert")).toHaveTextContent("invalid password");
  });

  it("renders actionable API list errors", async () => {
    server.use(
      http.get("/api/sessions", () =>
        HttpResponse.json({ ok: false, error: "database unavailable" }, { status: 503 }),
      ),
    );

    renderApp("/sessions");

    expect(await screen.findByText("database unavailable")).toBeInTheDocument();
  });

  it("treats failed sessions as attention states", () => {
    expect(isAttentionStatus("failed")).toBe(true);
    expect(isAttentionStatus("ready")).toBe(false);
  });

  it("does not render failed as a completed lifecycle", async () => {
    const { container } = renderApp("/sessions/sess_joinable_failed?scenario=failed");

    expect(
      await screen.findByText("Lifecycle stopped before successful completion.", { exact: false }),
    ).toBeInTheDocument();
    const completedSteps = Array.from(container.querySelectorAll(".life-step.done strong")).map(
      (step) => step.textContent,
    );

    expect(completedSteps).not.toContain("ended");
    expect(completedSteps).not.toContain("tearing_down");
    expect(completedSteps).not.toContain("awaiting_manual_download");
  });

  it("renders operational identifiers on the session detail route", async () => {
    renderApp("/sessions/sess_joinable_ready?scenario=ready");

    expect((await screen.findAllByText("sess_joinable_ready")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("joinable").length).toBeGreaterThan(0);
    expect(screen.getAllByText("123456789").length).toBeGreaterThan(0);
    expect(screen.getAllByText("203.0.113.10").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/^room-[a-z2-7]{26}\.sessions\.localhost$/).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText("dns_ready")).toBeInTheDocument();
  });

  it("renders timeline events in API order and access token last-used state", async () => {
    const session = makeSession({ id: "sess_detail", slug: "detail", title: "Detail" });
    const events = [
      makeEvent(session, { id: 1, type: "session.created", message: "Created" }),
      makeEvent(session, { id: 2, type: "droplet.create.started", message: "Creating droplet" }),
      makeEvent(session, { id: 3, type: "dns.create.succeeded", message: "DNS ready" }),
      makeEvent(session, { id: 4, type: "session.ready", message: "Ready" }),
    ];
    server.use(
      http.get("/api/sessions/sess_detail", () =>
        HttpResponse.json(
          makeDetail({
            session,
            events,
            access_tokens: [
              makeAccessToken(session, "host", {
                label: "Host link",
                last_used_at: "2026-04-25T10:01:00.000000000Z",
              }),
              makeAccessToken(session, "guest", {
                label: "Guest link",
                revoked_at: "2026-04-25T11:00:00.000000000Z",
              }),
            ],
          }),
        ),
      ),
    );

    renderApp("/sessions/sess_detail");

    expect(await screen.findByRole("heading", { name: "Detail" })).toBeInTheDocument();
    const eventTypes = screen
      .getAllByText(
        /^(session\.created|droplet\.create\.started|dns\.create\.succeeded|session\.ready)$/,
      )
      .map((item) => item.textContent);
    expect(eventTypes).toEqual([
      "session.created",
      "droplet.create.started",
      "dns.create.succeeded",
      "session.ready",
    ]);
    expect(screen.getByText("host")).toBeInTheDocument();
    expect(screen.getByText("guest")).toBeInTheDocument();
    expect(screen.getByText("Host link · active")).toBeInTheDocument();
    expect(screen.getByText("Guest link · revoked")).toBeInTheDocument();
    expect(screen.getByText(/Last used Apr 25, 2026 .*:01 AM/)).toBeInTheDocument();
    expect(screen.getByText(/Revoked Apr 25, 2026 .*:00 AM/)).toBeInTheDocument();
  });

  it("does not fabricate join links or timeline events on existing session details", async () => {
    const session = makeSession({ id: "sess_audit", slug: "audit", title: "Audit" });
    server.use(
      http.get("/api/sessions/sess_audit", () =>
        HttpResponse.json(
          makeDetail({
            session,
            events: [makeEvent(session, { id: 7, type: "session.created", message: "Created" })],
          }),
        ),
      ),
    );

    renderApp("/sessions/sess_audit");

    expect(await screen.findByRole("heading", { name: "Audit" })).toBeInTheDocument();
    expect(
      screen.getByText("Raw host and guest join links are shown only", { exact: false }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Host join link")).not.toBeInTheDocument();
    expect(screen.queryByText("Guest join link")).not.toBeInTheDocument();
    expect(screen.getByText("session.created")).toBeInTheDocument();
    expect(screen.queryByText("droplet.create.succeeded")).not.toBeInTheDocument();
    expect(screen.queryByText("session.ready")).not.toBeInTheDocument();
    expect(screen.queryByText("heartbeat")).not.toBeInTheDocument();
  });

  it("renders failed session error details", async () => {
    renderApp("/sessions/sess_joinable_failed?scenario=failed");

    expect(await screen.findByText("droplet health check failed")).toBeInTheDocument();
    expect(screen.getByText("health_check")).toBeInTheDocument();
  });

  it("renders manual download instructions when recordings await host download", async () => {
    renderApp("/sessions/sess_joinable_awaiting_manual_download?scenario=awaiting_manual_download");

    expect(await screen.findByText("Manual download required")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Download recordings" })).toHaveAttribute(
      "href",
      "https://room-awaiting-manual-download.sessions.localhost/downloads/session.zip",
    );
  });

  it("rejects unknown session statuses at the API schema boundary", () => {
    const payload = { ...makeSession(), status: "mystery_status" };

    expect(SessionSchema.safeParse(payload).success).toBe(false);
  });

  it("keeps the selected mock scenario after navigating to session detail", async () => {
    renderApp("/sessions?scenario=joinable");

    const link = await screen.findByRole("link", { name: "Joinable" });
    expect(link).toHaveAttribute("href", "/sessions/sess_joinable?scenario=joinable");
    fireEvent.click(link);

    expect(await screen.findByRole("heading", { name: "Joinable" })).toBeInTheDocument();
    expect(screen.getAllByText("sess_joinable").length).toBeGreaterThan(0);
  });

  it("serves the default mixed scenario join link", async () => {
    const response = await fetch("/api/join/joinable?token=guest-token");

    expect(response.status).toBe(200);
  });

  it("rejects duplicate mock session slugs", async () => {
    const response = await fetch("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: "Joinable", slug: "joinable" }),
    });

    expect(response.status).toBe(409);
    expect(await response.json()).toMatchObject({ error: "session slug already exists" });
  });

  it("renders the join waiting state for a valid guest token", async () => {
    renderApp("/join/joinable?token=guest-token&scenario=provisioning");

    expect(await screen.findByText("Provisioning your room")).toBeInTheDocument();
    expect(screen.getByText("You're joining as")).toBeInTheDocument();
    expect(screen.getByText("guest")).toBeInTheDocument();
  });

  it("renders the join waiting state for a valid host token", async () => {
    renderApp("/join/joinable?token=host-token&scenario=waiting_for_dns");

    expect(await screen.findByText("Provisioning your room")).toBeInTheDocument();
    expect(screen.getByText("Polling every 5 seconds")).toBeInTheDocument();
    expect(screen.getByText("host")).toBeInTheDocument();
  });

  it("renders ready join links without pretending to keep polling", async () => {
    renderApp("/join/joinable?token=guest-token&scenario=ready");

    expect(await screen.findByText("Room is ready")).toBeInTheDocument();
    expect(screen.getByText("Room is ready.")).toBeInTheDocument();
    expect(screen.queryByText("Polling every 5 seconds")).not.toBeInTheDocument();
    expect(screen.queryByText("Provisioning your room")).not.toBeInTheDocument();
  });

  it("renders failed join links as unavailable without waiting UI", async () => {
    renderApp("/join/joinable?token=guest-token&scenario=failed");

    expect(await screen.findByText("Session unavailable")).toBeInTheDocument();
    expect(
      screen.getByText("This session failed before it became joinable.", { exact: false }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Polling every 5 seconds")).not.toBeInTheDocument();
    expect(screen.queryByText("Creating droplet")).not.toBeInTheDocument();
  });

  it("renders invalid join tokens from the API", async () => {
    renderApp("/join/joinable?token=bad-token");

    expect(await screen.findByText("join link not found")).toBeInTheDocument();
  });

  it("renders invalid join slugs from the API", async () => {
    renderApp("/join/not-real?token=guest-token&scenario=provisioning");

    expect(await screen.findByText("join link not found")).toBeInTheDocument();
  });

  it("submits title and slug when creating a session", async () => {
    let posted: unknown;
    server.use(
      http.post("/api/sessions", async ({ request }) => {
        posted = await request.json();
        const session = makeSession({
          id: "sess_recorded_post",
          slug: "recorded-post",
          title: "Recorded Post",
        });
        return HttpResponse.json(
          {
            session,
            join_links: {
              host: { url: "http://127.0.0.1:5173/join/recorded-post?token=host", role: "host" },
              guest: {
                url: "http://127.0.0.1:5173/join/recorded-post?token=guest",
                role: "guest",
              },
            },
            events: [],
            tokens: {
              host: { id: "sat_host", token: "host" },
              guest: { id: "sat_guest", token: "guest" },
            },
          },
          { status: 201 },
        );
      }),
    );
    renderApp("/sessions/new");

    fireEvent.change(await screen.findByLabelText("Session title"), {
      target: { value: "Recorded Post" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "+ Create session" }).closest("form")!);

    await waitFor(() =>
      expect(posted).toMatchObject({ title: "Recorded Post", slug: "recorded-post" }),
    );
    expect(await screen.findByRole("heading", { name: "Recorded Post" })).toBeInTheDocument();
  });

  it("renders one-time join links after creating a session", async () => {
    renderApp("/sessions/new");

    fireEvent.change(await screen.findByLabelText("Session title"), {
      target: { value: "New Mock Session" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "+ Create session" }).closest("form")!);

    expect(await screen.findByRole("heading", { name: "New Mock Session" })).toBeInTheDocument();
    expect(
      screen.getByText("Raw join links are shown once.", { exact: false }),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText(
        /^https?:\/\/[^/]+\/join\/new-mock-session\?token=new-mock-session-host-token$/,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText(
        /^https?:\/\/[^/]+\/join\/new-mock-session\?token=new-mock-session-guest-token$/,
      ).length,
    ).toBeGreaterThan(0);
  });

  it("renders duplicate slug validation errors from the server", async () => {
    renderApp("/sessions/new");

    fireEvent.change(await screen.findByLabelText("Session title"), {
      target: { value: "Joinable" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "+ Create session" }).closest("form")!);

    expect(await screen.findByText("session slug already exists")).toBeInTheDocument();
  });

  it("scopes mock join tokens to the created session", async () => {
    await fetch("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: "Scoped Token" }),
    });

    const wrongToken = await fetch("/api/join/scoped-token?token=guest-token");
    expect(wrongToken.status).toBe(404);

    const scopedToken = await fetch("/api/join/scoped-token?token=scoped-token-guest-token");
    expect(scopedToken.status).toBe(200);
  });

  it("renders missing join tokens without calling the API", async () => {
    let requests = 0;
    server.use(
      http.get("/api/join/:slug", () => {
        requests += 1;
        return HttpResponse.json({ ok: false, error: "unexpected join request" }, { status: 500 });
      }),
    );

    renderApp("/join/joinable");

    expect(screen.getByText("Join link is missing its token.")).toBeInTheDocument();
    expect(requests).toBe(0);
  });

  it("centralizes polling decisions around non-terminal session statuses", () => {
    expect(sessionsListRefetchInterval({ sessions: [makeSession({ status: "failed" })] })).toBe(
      false,
    );
    expect(
      sessionsListRefetchInterval({ sessions: [makeSession({ status: "provisioning" })] }),
    ).toBe(5_000);
    expect(
      sessionDetailRefetchInterval({ ...makeDetail(), session: makeSession({ status: "ended" }) }),
    ).toBe(false);
    expect(
      sessionDetailRefetchInterval({ ...makeDetail(), session: makeSession({ status: "active" }) }),
    ).toBe(5_000);
    expect(
      joinRefetchInterval(makeJoinResponse({ session: makeSession({ status: "failed" }) })),
    ).toBe(false);
    expect(
      joinRefetchInterval(
        makeJoinResponse({ session: makeSession({ status: "waiting_for_dns" }) }),
      ),
    ).toBe(5_000);
  });
});

async function fetchAuthSession() {
  const response = await fetch("/api/auth/session");
  return response.json();
}

function statValue(label: string) {
  return within(screen.getByRole("group", { name: label })).getByText(/^\d+$/).textContent;
}
