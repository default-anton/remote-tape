import { fireEvent, screen } from "@testing-library/react";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { createControlMockApi } from "./testing/handlers";
import { renderApp } from "./testing/renderApp";

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

  it("keeps the selected mock scenario after navigating to session detail", async () => {
    renderApp("/sessions?scenario=joinable");

    const link = await screen.findByRole("link", { name: "Joinable" });
    expect(link).toHaveAttribute("href", "/sessions/sess_joinable?scenario=joinable");
    fireEvent.click(link);

    expect(await screen.findByRole("heading", { name: "Joinable" })).toBeInTheDocument();
    expect(screen.getByText("sess_joinable")).toBeInTheDocument();
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
    expect(screen.getByText("host")).toBeInTheDocument();
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
    renderApp("/sessions");

    fireEvent.change(await screen.findByLabelText("Session title"), {
      target: { value: "New Mock Session" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "+ Create session" }).closest("form")!);

    expect(await screen.findByRole("heading", { name: "New Mock Session" })).toBeInTheDocument();
    expect(
      screen.getByText(
        /^https?:\/\/[^/]+\/join\/new-mock-session\?token=new-mock-session-host-token$/,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /^https?:\/\/[^/]+\/join\/new-mock-session\?token=new-mock-session-guest-token$/,
      ),
    ).toBeInTheDocument();
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
    renderApp("/join/joinable");

    expect(screen.getByText("Join link is missing its token.")).toBeInTheDocument();
  });
});
