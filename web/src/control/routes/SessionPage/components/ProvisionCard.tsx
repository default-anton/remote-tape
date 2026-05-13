import type { ProvisioningOptions } from "../../../types";
import { Icon } from "../../../components/Icon";

export type ProvisioningSelection = {
  region: string;
  size: string;
};

export function ProvisionCard({
  options,
  selection,
}: {
  options: ProvisioningOptions | undefined;
  selection: ProvisioningSelection;
}) {
  const items = [
    ["do", "DigitalOcean instance", instanceCopy(options, selection)],
    ["cf", "Cloudflare DNS", "DNS record for the session server domain derived from your slug."],
    [
      "room",
      "Session room",
      "Disposable room app, LiveKit/TURN, and recording ingest on the session server.",
    ],
    [
      "sync",
      "Background Reconciler",
      "Continuously monitors and maintains your session resources.",
    ],
  ];
  return (
    <aside className="panel provision-card">
      <h2>What will be provisioned</h2>
      {items.map(([tone, title, copy]) => (
        <div className="provision-item" key={title}>
          <span className={`round-icon ${tone}`}>
            <Icon
              name={
                tone === "do"
                  ? "digitalOcean"
                  : tone === "cf"
                    ? "cloud"
                    : tone === "room"
                      ? "play"
                      : "refresh"
              }
            />
          </span>
          <div>
            <strong>{title}</strong>
            <p>{copy}</p>
          </div>
        </div>
      ))}
      <div className="provision-note">
        ⓘ You’ll be redirected to the session once it’s ready. This usually takes 2–5 minutes.
      </div>
    </aside>
  );
}

function instanceCopy(options: ProvisioningOptions | undefined, selection: ProvisioningSelection) {
  if (!options) return "A per-session DigitalOcean droplet using the selected region and size.";

  const region = options.regions.find((candidate) => candidate.slug === selection.region);
  const size = options.sizes.find((candidate) => candidate.slug === selection.size);
  if (!region || !size) return "A per-session DigitalOcean droplet using valid catalog options.";

  const cpuClass = size.dedicated_cpu ? "dedicated CPU" : "shared CPU";
  return `A ${cpuClass} DigitalOcean droplet in ${region.label} (${region.slug}) sized ${size.slug}: ${size.description}.`;
}
