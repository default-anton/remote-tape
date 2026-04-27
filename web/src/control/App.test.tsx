import { fireEvent, screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import {
  joinRefetchInterval,
  sessionDetailRefetchInterval,
  sessionsListRefetchInterval,
} from "./api/hooks";
import { isAttentionStatus } from "./domain/sessionStatus";
import { makeDetail, makeJoinResponse, makeSession } from "./testing/fixtures";
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

  it("renders the detail page after creating a session in the mock API", async () => {
    renderApp("/sessions/new");

    fireEvent.change(await screen.findByLabelText("Session title"), {
      target: { value: "New Mock Session" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "+ Create session" }).closest("form")!);

    expect(await screen.findByRole("heading", { name: "New Mock Session" })).toBeInTheDocument();
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
