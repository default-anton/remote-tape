import type { ListSessionEventsParams, ListSessionsParams } from "./client";

export const sessionsKeys = {
  all: ["sessions"] as const,
  list: (params?: ListSessionsParams) => [...sessionsKeys.all, "list", params ?? {}] as const,
  detail: (id: string) => [...sessionsKeys.all, "detail", id] as const,
  events: (id: string, params?: ListSessionEventsParams) =>
    [...sessionsKeys.all, "events", id, params ?? {}] as const,
};

export const slugKeys = {
  all: ["session-slugs"] as const,
  detail: (slug: string) => [...slugKeys.all, slug] as const,
};

export const joinKeys = {
  all: ["join"] as const,
  detail: (slug: string, token: string) => [...joinKeys.all, slug, token] as const,
};
