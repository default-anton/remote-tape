import { type ReactNode } from "react";

export function Field({
  label,
  help,
  count,
  children,
}: {
  label: string;
  help: string;
  count?: string;
  children: ReactNode;
}) {
  const id =
    label === "Session title"
      ? "title"
      : label === "Session slug"
        ? "slug"
        : label === "Preferred region"
          ? "region"
          : label === "Droplet size"
            ? "size"
            : undefined;
  return (
    <div className="field">
      <div className="field-head">
        <label htmlFor={id}>{label}</label>
      </div>
      <p>{help}</p>
      {children}
      {count ? <span className="field-count">{count}</span> : null}
    </div>
  );
}
