import type { Session } from "../types";

export function domainFor(session: Session) {
  return session.room_domain ?? `${session.slug}.cast.remote-tape.io`;
}

export function value(input: string | null) {
  return input && input.length > 0 ? input : "—";
}
