export const sessionsKeys = {
  all: ["sessions"] as const,
  list: () => sessionsKeys.all,
  detail: (id: string) => [...sessionsKeys.all, id] as const,
};

export const joinKeys = {
  all: ["join"] as const,
  detail: (slug: string, token: string) => [...joinKeys.all, slug, token] as const,
};
