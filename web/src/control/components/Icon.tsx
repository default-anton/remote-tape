import { type ReactNode } from "react";

export type IconName =
  | "activity"
  | "calendar"
  | "check"
  | "chevronDown"
  | "chevronLeft"
  | "chevronRight"
  | "cloud"
  | "copy"
  | "digitalOcean"
  | "download"
  | "filter"
  | "infinity"
  | "more"
  | "play"
  | "refresh"
  | "search"
  | "square"
  | "spinner"
  | "triangle";

export function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, ReactNode> = {
    activity: <polyline points="2 12 5 12 8 4 12 20 16 8 19 12 22 12" />,
    calendar: (
      <>
        <path d="M8 2v4M16 2v4M3 10h18" />
        <rect x="3" y="5" width="18" height="16" rx="2" />
      </>
    ),
    check: <path d="m5 12 4 4L19 6" />,
    chevronDown: <path d="m6 9 6 6 6-6" />,
    chevronLeft: <path d="m15 18-6-6 6-6" />,
    chevronRight: <path d="m9 18 6-6-6-6" />,
    cloud: <path d="M17.5 18H7a5 5 0 0 1 .7-9.95A6 6 0 0 1 19 10.5 3.75 3.75 0 0 1 17.5 18z" />,
    copy: (
      <>
        <rect x="9" y="9" width="11" height="11" rx="2" />
        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
      </>
    ),
    digitalOcean: (
      <>
        <path d="M12 3a9 9 0 0 1 9 9 9 9 0 0 1-9 9h-2v-4h2a5 5 0 1 0-5-5v5H3v-5a9 9 0 0 1 9-9z" />
        <path d="M3 21h4v-4H3z" />
      </>
    ),
    download: (
      <>
        <path d="M12 3v12" />
        <path d="m7 10 5 5 5-5" />
        <path d="M5 21h14" />
      </>
    ),
    filter: <path d="M4 5h16l-6 7v5l-4 2v-7z" />,
    infinity: (
      <path d="M8.5 8.5c2.5 0 4.5 7 7 7a3.5 3.5 0 1 0 0-7c-2.5 0-4.5 7-7 7a3.5 3.5 0 1 1 0-7z" />
    ),
    more: (
      <>
        <circle cx="12" cy="5" r="1" />
        <circle cx="12" cy="12" r="1" />
        <circle cx="12" cy="19" r="1" />
      </>
    ),
    play: <path d="m8 5 11 7-11 7z" />,
    refresh: (
      <>
        <path d="M20 11a8 8 0 1 0-2.34 5.66" />
        <path d="M20 5v6h-6" />
      </>
    ),
    search: (
      <>
        <circle cx="11" cy="11" r="7" />
        <path d="m20 20-3.5-3.5" />
      </>
    ),
    square: <rect x="7" y="7" width="10" height="10" rx="1" />,
    spinner: (
      <>
        <path d="M21 12a9 9 0 0 1-9 9" />
        <path d="M3 12a9 9 0 0 1 9-9" />
      </>
    ),
    triangle: (
      <>
        <path d="M12 3 22 20H2z" />
        <path d="M12 9v5" />
        <path d="M12 17h.01" />
      </>
    ),
  };
  return (
    <svg className="icon" viewBox="0 0 24 24" aria-hidden="true">
      {paths[name]}
    </svg>
  );
}
