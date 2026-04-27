import { type IconName } from "../../../components/Icon";
import { type SessionStatus } from "../../../domain/sessionStatus";

export function actionIcon(status: SessionStatus): IconName {
  switch (status) {
    case "active":
      return "square";
    case "finalizing":
    case "awaiting_manual_download":
    case "ended":
      return "download";
    case "teardown_pending":
    case "tearing_down":
    case "failed":
      return "refresh";
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "ready":
      return "play";
    default:
      return exhaustiveStatus(status);
  }
}

function exhaustiveStatus(status: never): never {
  throw new Error(`Unhandled session status: ${status}`);
}
