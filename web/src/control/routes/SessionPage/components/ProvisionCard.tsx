import { Icon } from "../../../components/Icon";

export function ProvisionCard() {
  const items = [
    [
      "do",
      "DigitalOcean instance",
      "A dedicated instance in US East 1 (New York) sized 2 vCPU / 4 GB RAM.",
    ],
    [
      "cf",
      "Cloudflare DNS",
      "DNS record for the-infra-podcast-313.remote-tape.io proxied via Cloudflare.",
    ],
    [
      "link",
      "Stable Join Links",
      "Persistent, shareable links for your guests that remain stable across restarts.",
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
                    : tone === "link"
                      ? "infinity"
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
