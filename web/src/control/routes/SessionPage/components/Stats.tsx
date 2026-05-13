import { Icon, type IconName } from "../../../components/Icon";
import type { SessionsResponse } from "../../../types";

export function Stats({ summary }: { summary: SessionsResponse["summary"] | undefined }) {
  const counts = summary ?? {
    total: 0,
    provisioning: 0,
    ready: 0,
    active: 0,
    awaiting_manual_download: 0,
    failed: 0,
  };

  return (
    <section className="stats mock-stats" aria-label="Session summary">
      <div className="stats-title">Sessions</div>
      <div className="stat-pills">
        <Stat tone="gray" icon="activity" label="Total" value={counts.total} />
        <Stat tone="orange" icon="spinner" label="Provisioning" value={counts.provisioning} />
        <Stat tone="green" icon="check" label="Ready" value={counts.ready} />
        <Stat tone="blue" icon="activity" label="Active" value={counts.active} />
        <Stat
          tone="purple"
          icon="download"
          label="Awaiting download"
          value={counts.awaiting_manual_download}
        />
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
