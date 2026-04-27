import { z } from "zod";

export const SESSION_STATUSES = [
  "created",
  "provisioning",
  "waiting_for_dns",
  "ready",
  "active",
  "finalizing",
  "awaiting_manual_download",
  "teardown_pending",
  "tearing_down",
  "ended",
  "failed",
] as const;

export const SessionStatusSchema = z.enum(SESSION_STATUSES);
export type SessionStatus = z.infer<typeof SessionStatusSchema>;
export type SessionLifecycleStatus = Exclude<SessionStatus, "failed">;

export const SESSION_LIFECYCLE_STATUSES = [
  "created",
  "provisioning",
  "waiting_for_dns",
  "ready",
  "active",
  "finalizing",
  "awaiting_manual_download",
  "teardown_pending",
  "tearing_down",
  "ended",
] as const satisfies readonly SessionLifecycleStatus[];

export type SessionStatusTone = "orange" | "yellow" | "green" | "blue" | "purple" | "red" | "gray";

export function sessionStatusLabel(status: SessionStatus): string {
  switch (status) {
    case "created":
      return "created";
    case "provisioning":
      return "provisioning";
    case "waiting_for_dns":
      return "waiting_for_dns";
    case "ready":
      return "ready";
    case "active":
      return "active";
    case "finalizing":
      return "finalizing";
    case "awaiting_manual_download":
      return "awaiting_manual_download";
    case "teardown_pending":
      return "teardown_pending";
    case "tearing_down":
      return "tearing_down";
    case "ended":
      return "ended";
    case "failed":
      return "failed";
    default:
      return exhaustiveStatus(status);
  }
}

export function sessionStatusClassName(status: SessionStatus): string {
  return `status-${status.replaceAll("_", "-")}`;
}

export function sessionStatusTone(status: SessionStatus): SessionStatusTone {
  switch (status) {
    case "created":
    case "provisioning":
      return "orange";
    case "waiting_for_dns":
    case "teardown_pending":
    case "tearing_down":
      return "yellow";
    case "ready":
      return "green";
    case "active":
      return "blue";
    case "finalizing":
    case "awaiting_manual_download":
      return "purple";
    case "failed":
      return "red";
    case "ended":
      return "gray";
    default:
      return exhaustiveStatus(status);
  }
}

export function isProvisioningLikeStatus(status: SessionStatus): boolean {
  switch (status) {
    case "created":
    case "provisioning":
    case "waiting_for_dns":
      return true;
    case "ready":
    case "active":
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
    case "tearing_down":
    case "ended":
    case "failed":
      return false;
    default:
      return exhaustiveStatus(status);
  }
}

export function isJoinRedirectReadyStatus(status: SessionStatus): boolean {
  switch (status) {
    case "ready":
    case "active":
      return true;
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
    case "tearing_down":
    case "ended":
    case "failed":
      return false;
    default:
      return exhaustiveStatus(status);
  }
}

export function isTerminalStatus(status: SessionStatus): boolean {
  switch (status) {
    case "ended":
    case "failed":
      return true;
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "ready":
    case "active":
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
    case "tearing_down":
      return false;
    default:
      return exhaustiveStatus(status);
  }
}

export function isAttentionStatus(status: SessionStatus): boolean {
  switch (status) {
    case "failed":
      return true;
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "ready":
    case "active":
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
    case "tearing_down":
    case "ended":
      return false;
    default:
      return exhaustiveStatus(status);
  }
}

export function shouldPollSession(status: SessionStatus): boolean {
  switch (status) {
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "ready":
    case "active":
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
    case "tearing_down":
      return true;
    case "ended":
    case "failed":
      return false;
    default:
      return exhaustiveStatus(status);
  }
}

export function shouldPollJoin(status: SessionStatus): boolean {
  switch (status) {
    case "created":
    case "provisioning":
    case "waiting_for_dns":
      return true;
    case "ready":
    case "active":
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
    case "tearing_down":
    case "ended":
    case "failed":
      return false;
    default:
      return exhaustiveStatus(status);
  }
}

export function sessionLifecycleIndex(status: SessionStatus): number | null {
  switch (status) {
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "ready":
    case "active":
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
    case "tearing_down":
    case "ended":
      return SESSION_LIFECYCLE_STATUSES.indexOf(status);
    case "failed":
      return null;
    default:
      return exhaustiveStatus(status);
  }
}

function exhaustiveStatus(status: never): never {
  throw new Error(`Unhandled session status: ${status}`);
}
