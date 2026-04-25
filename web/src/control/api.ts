import type { z } from "zod";
import {
  CreateSessionResponseSchema,
  DetailSchema,
  JoinResponseSchema,
  SessionsResponseSchema,
  type CreateSessionInput,
} from "./types";

async function requestJSON<T>(path: string, schema: z.ZodType<T>, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: init?.body ? { "Content-Type": "application/json", ...init.headers } : init?.headers,
    ...init,
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
