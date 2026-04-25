import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import { MemoryRouter } from "react-router";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { App } from "./App";
import type { Session } from "./types";

const session: Session = {
  id: "sess_123",
  slug: "joinable",
  title: "Joinable",
  status: "created",
  droplet_id: null,
  droplet_ip: null,
  droplet_region: "nyc3",
  droplet_size: "s-2vcpu-2gb",
  image_id: "ubuntu-24-04-x64",
  room_domain: "room-abc.sessions.localhost",
  dns_record_id: null,
  livekit_url: null,
  recording_download_url: null,
  finalization_summary_json: null,
  created_at: "2026-04-25T10:00:00.000000000Z",
  updated_at: "2026-04-25T10:00:00.000000000Z",
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
};

const server = setupServer(
  http.get("/api/sessions", () => HttpResponse.json({ sessions: [session] })),
  http.get("/api/join/:slug", ({ request, params }) => {
    const url = new URL(request.url);
    if (params.slug !== "joinable" || url.searchParams.get("token") !== "guest-token") {
      return HttpResponse.json({ ok: false, error: "join link not found" }, { status: 404 });
    }
    return HttpResponse.json({
      session: {
        slug: session.slug,
        title: session.title,
        status: session.status,
      },
      token: {
        role: "guest",
      },
    });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

describe("control app", () => {
  it("renders sessions from the API", async () => {
    renderApp("/sessions");

    expect(await screen.findByRole("link", { name: "Joinable" })).toBeInTheDocument();
    expect(screen.getByText("room-abc.sessions.localhost")).toBeInTheDocument();
  });

  it("renders the join waiting state for a valid token", async () => {
    renderApp("/join/joinable?token=guest-token");

    expect(await screen.findByText("Provisioning your room")).toBeInTheDocument();
    expect(screen.getByText("You're joining as")).toBeInTheDocument();
    expect(screen.getByText("guest")).toBeInTheDocument();
  });
});

function renderApp(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
