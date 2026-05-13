import type { z } from "zod";
import { redirectToLogin, shouldRedirectUnauthorized } from "../authRedirect";
import {
  AuthSessionSchema,
  CreateSessionResponseSchema,
  DetailSchema,
  JoinResponseSchema,
  SessionsResponseSchema,
  SlugAvailabilitySchema,
  type CreateSessionInput,
} from "../types";

let csrfToken: string | undefined;

async function requestJSON<T>(path: string, schema: z.ZodType<T>, init?: RequestInit): Promise<T> {
  const response = await fetchWithCSRF(path, init);
  const payload = await responsePayload(response);

  if (!response.ok) {
    if (shouldRedirectUnauthorized(path, response.status)) redirectToLogin();
    const message = errorMessage(payload) ?? `${response.status} ${response.statusText}`;
    throw new Error(message);
  }
  return schema.parse(payload);
}

async function fetchWithCSRF(path: string, init?: RequestInit, retried = false): Promise<Response> {
  const headers = new Headers(init?.headers);
  const unsafe = isUnsafeMethod(init?.method);
  if (init?.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  if (unsafe) headers.set("X-CSRF-Token", await getCSRFToken());

  const response = await fetch(path, {
    ...init,
    headers,
  });
  if (response.status !== 403 || !unsafe || retried) return response;

  csrfToken = undefined;
  return fetchWithCSRF(path, init, true);
}

async function responsePayload(response: Response) {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

async function getCSRFToken() {
  if (csrfToken) return csrfToken;
  const session = await authSession();
  csrfToken = session.csrf_token;
  return csrfToken;
}

function isUnsafeMethod(method = "GET") {
  return !["GET", "HEAD", "OPTIONS", "TRACE"].includes(method.toUpperCase());
}

function errorMessage(payload: unknown): string | undefined {
  if (payload && typeof payload === "object" && "error" in payload) {
    const value = payload.error;
    if (typeof value === "string" && value.length > 0) {
      return value;
    }
  }
  return undefined;
}

export function authSession() {
  return requestJSON("/api/auth/session", AuthSessionSchema);
}

export async function login(password: string) {
  await postForm("/login", { password });
}

export async function logout() {
  await postForm("/logout", {});
  csrfToken = undefined;
}

async function postForm(path: string, fields: Record<string, string>) {
  const body = new URLSearchParams(fields);
  const response = await fetchWithCSRF(path, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
  if (response.ok) return;

  const payload = await responsePayload(response);
  const message = errorMessage(payload) ?? `${response.status} ${response.statusText}`;
  throw new Error(message);
}

export type ListSessionsParams = {
  page?: number;
  pageSize?: number;
  sort?: string;
  direction?: "asc" | "desc";
  status?: string;
  region?: string;
  query?: string;
};

export function listSessions(params: ListSessionsParams = {}) {
  const query = new URLSearchParams();
  if (params.page) query.set("page", String(params.page));
  if (params.pageSize) query.set("page_size", String(params.pageSize));
  if (params.sort) query.set("sort", params.sort);
  if (params.direction) query.set("direction", params.direction);
  if (params.status) query.set("status", params.status);
  if (params.region) query.set("region", params.region);
  if (params.query) query.set("q", params.query);
  const suffix = query.size > 0 ? `?${query}` : "";
  return requestJSON(`/api/sessions${suffix}`, SessionsResponseSchema);
}

export function getSession(id: string) {
  return requestJSON(`/api/sessions/${encodeURIComponent(id)}`, DetailSchema);
}

export function checkSlugAvailability(slug: string) {
  return requestJSON(`/api/session-slugs/${encodeURIComponent(slug)}`, SlugAvailabilitySchema);
}

export function createSession(input: CreateSessionInput) {
  return requestJSON("/api/sessions", CreateSessionResponseSchema, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function forceDestroySessionServer(id: string, confirmation: string) {
  return requestJSON(`/api/sessions/${encodeURIComponent(id)}/force-destroy`, DetailSchema, {
    method: "POST",
    body: JSON.stringify({ confirmation }),
  });
}

export function joinSession(slug: string, token: string) {
  const params = new URLSearchParams({ token });
  return requestJSON(`/api/join/${encodeURIComponent(slug)}?${params}`, JoinResponseSchema);
}
