import { Shell } from "./Layout";

export function PlaceholderPage({ title }: { title: string }) {
  return (
    <Shell active={title === "Settings" ? "settings" : "diagnostics"}>
      <h1>{title}</h1>
      <p className="lead">Mock screen scaffold. Implementation lands in a later slice.</p>
    </Shell>
  );
}
