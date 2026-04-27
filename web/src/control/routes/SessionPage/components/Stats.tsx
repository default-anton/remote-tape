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
    <div className="stats mock-stats">
      <Stat tone="blue" icon="activity" label="Total" value={sessions.length} hint="All sessions" />
      <Stat
        tone="orange"
        icon="spinner"
        label="Provisioning"
        value={counts.provisioning}
        hint="Creating resources"
      />
      <Stat tone="green" icon="check" label="Ready" value={counts.ready} hint="Ready to start" />
      <Stat
        tone="blue"
        icon="activity"
        label="Active"
        value={counts.active}
        hint="Currently recording"
      />
      <Stat
        tone="purple"
        icon="download"
        label="Awaiting download"
        value={counts.awaiting}
        hint="Recording complete"
      />
      <Stat
        tone="red"
        icon="triangle"
        label="Failed"
        value={counts.failed}
        hint="Needs attention"
      />
    </div>
  );
}

function Stat({
  tone,
  icon,
  label,
  value,
  hint,
}: {
  tone: string;
  icon: IconName;
  label: string;
  value: number;
  hint: string;
}) {
  return (
    <div className="stat cardish" role="group" aria-label={`${label} sessions`}>
      <div className={`round-icon ${tone}`}>
        <Icon name={icon} />
      </div>
      <div className="stat-label">{label}</div>
      <b>{value}</b>
      <span className="muted">{hint}</span>
      <svg className={`spark ${tone}`} viewBox="0 0 120 20" aria-hidden="true">
        <path d="M2 12 C18 12 18 7 34 10 S50 16 66 9 82 6 98 12 112 11 118 8" />
      </svg>
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
