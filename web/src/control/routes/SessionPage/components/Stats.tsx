import { useMemo } from "react";
import { Icon, type IconName } from "../../../components/Icon";
import {
  isAttentionStatus,
  isProvisioningLikeStatus,
  type SessionStatus,
} from "../../../domain/sessionStatus";
import type { Session } from "../../../types";

export function Stats({ sessions }: { sessions: Session[] }) {
  const counts = useMemo(() => {
    return sessions.reduce(
      (acc, session) => {
        if (isProvisioningLikeStatus(session.status)) acc.provisioning += 1;
        if (session.status === "ready") acc.ready += 1;
        if (session.status === "active") acc.active += 1;
        if (isAwaitingDownloadLikeStatus(session.status)) acc.awaiting += 1;
        if (isAttentionStatus(session.status)) acc.failed += 1;
        return acc;
      },
      { provisioning: 0, ready: 0, active: 0, awaiting: 0, failed: 0 },
    );
  }, [sessions]);

  return (
    <section className="stats mock-stats" aria-label="Session summary">
      <div className="stats-title">Sessions</div>
      <div className="stat-pills">
        <Stat tone="gray" icon="activity" label="Total" value={sessions.length} />
        <Stat tone="orange" icon="spinner" label="Provisioning" value={counts.provisioning} />
        <Stat tone="green" icon="check" label="Ready" value={counts.ready} />
        <Stat tone="blue" icon="activity" label="Active" value={counts.active} />
        <Stat tone="purple" icon="download" label="Awaiting download" value={counts.awaiting} />
        <Stat tone="red" icon="triangle" label="Failed" value={counts.failed} />
      </div>
    </section>
  );
}

function Stat({
  tone,
  icon,
  label,
  value,
}: {
  tone: string;
  icon: IconName;
  label: string;
  value: number;
}) {
  return (
    <div className={`status stat-pill ${tone}`} role="group" aria-label={`${label} sessions`}>
      <Icon name={icon} />
      <span>{label}</span>
      <b>{value}</b>
    </div>
  );
}

function isAwaitingDownloadLikeStatus(status: SessionStatus): boolean {
  switch (status) {
    case "finalizing":
    case "awaiting_manual_download":
    case "teardown_pending":
      return true;
    case "created":
    case "provisioning":
    case "waiting_for_dns":
    case "ready":
    case "active":
    case "tearing_down":
    case "ended":
    case "failed":
      return false;
    default:
      return exhaustiveStatus(status);
  }
}

function exhaustiveStatus(status: never): never {
  throw new Error(`Unhandled session status: ${status}`);
}
