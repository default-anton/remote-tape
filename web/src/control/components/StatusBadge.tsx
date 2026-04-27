import {
  sessionStatusClassName,
  sessionStatusLabel,
  sessionStatusTone,
  type SessionStatus,
} from "../domain/sessionStatus";
import { Icon, type IconName } from "./Icon";

export function StatusBadge({ status, label }: { status: SessionStatus; label?: string }) {
  return (
    <span className={`status ${sessionStatusClassName(status)} ${sessionStatusTone(status)}`}>
      <b>
        <Icon name={statusIcon(status)} />
      </b>
      {label ?? sessionStatusLabel(status)}
    </span>
  );
}

export function statusIcon(status: SessionStatus): IconName {
  switch (status) {
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "finalizing":
      return "spinner";
    case "ready":
    case "ended":
      return "check";
    case "active":
      return "activity";
    case "awaiting_manual_download":
      return "download";
    case "teardown_pending":
      return "calendar";
    case "tearing_down":
      return "refresh";
    case "failed":
      return "triangle";
    default:
      return exhaustiveStatus(status);
  }
}

function exhaustiveStatus(status: never): never {
  throw new Error(`Unhandled session status: ${status}`);
}
