import type { ListSessionsParams } from "./client";

export const sessionsKeys = {
  all: ["sessions"] as const,
  list: (params?: ListSessionsParams) => [...sessionsKeys.all, "list", params ?? {}] as const,
  detail: (id: string) => [...sessionsKeys.all, "detail", id] as const,
};

export const slugKeys = {
  all: ["session-slugs"] as const,
  detail: (slug: string) => [...slugKeys.all, slug] as const,
};

export const joinKeys = {
  all: ["join"] as const,
  detail: (slug: string, token: string) => [...joinKeys.all, slug, token] as const,
};
