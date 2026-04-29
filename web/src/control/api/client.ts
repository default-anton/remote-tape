import type { z } from "zod";
import {
  AuthSessionSchema,
  CreateSessionResponseSchema,
  DetailSchema,
  JoinResponseSchema,
  SessionsResponseSchema,
  type CreateSessionInput,
} from "../types";

let csrfToken: string | undefined;

async function requestJSON<T>(path: string, schema: z.ZodType<T>, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  if (isUnsafeMethod(init?.method)) headers.set("X-CSRF-Token", await getCSRFToken());

  const response = await fetch(path, {
    ...init,
    headers,
  });

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    payload = null;
  }

  if (!response.ok) {
    const message = errorMessage(payload) ?? `${response.status} ${response.statusText}`;
    throw new Error(message);
  }
  return schema.parse(payload);
}

async function getCSRFToken() {
  if (csrfToken) return csrfToken;
  const session = await requestJSON("/api/auth/session", AuthSessionSchema);
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

export function listSessions() {
  return requestJSON("/api/sessions", SessionsResponseSchema);
}

export function getSession(id: string) {
  return requestJSON(`/api/sessions/${encodeURIComponent(id)}`, DetailSchema);
}

export function createSession(input: CreateSessionInput) {
  return requestJSON("/api/sessions", CreateSessionResponseSchema, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function joinSession(slug: string, token: string) {
  const params = new URLSearchParams({ token });
  return requestJSON(`/api/join/${encodeURIComponent(slug)}?${params}`, JoinResponseSchema);
}
