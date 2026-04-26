import { type ReactNode } from "react";
import { Link } from "react-router";

type NavIcon = "activity" | "diagnostics" | "server" | "settings" | "chevronDown" | "chevronRight";

function Icon({ name }: { name: NavIcon }) {
  const paths: Record<NavIcon, ReactNode> = {
    activity: <polyline points="2 12 5 12 8 4 12 20 16 8 19 12 22 12" />,
    diagnostics: (
      <>
        <path d="M12 2v7" />
        <path d="M8 11 4 18" />
        <path d="m16 11 4 7" />
        <circle cx="12" cy="11" r="3" />
        <circle cx="4" cy="19" r="2" />
        <circle cx="20" cy="19" r="2" />
      </>
    ),
    server: (
      <>
        <rect x="4" y="4" width="16" height="4" rx="1" />
        <rect x="4" y="10" width="16" height="4" rx="1" />
        <rect x="4" y="16" width="16" height="4" rx="1" />
      </>
    ),
    settings: (
      <>
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.12 2.12-.06-.06a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1.04 1.56V20h-3v-.08a1.7 1.7 0 0 0-1.04-1.56 1.7 1.7 0 0 0-1.88.34l-.06.06-2.12-2.12.06-.06A1.7 1.7 0 0 0 5 14.7 1.7 1.7 0 0 0 3.44 13H3v-3h.44A1.7 1.7 0 0 0 5 8.3a1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.12-2.12.06.06A1.7 1.7 0 0 0 8.66 4a1.7 1.7 0 0 0 1.04-1.56V2h3v.44A1.7 1.7 0 0 0 13.74 4a1.7 1.7 0 0 0 1.88-.34l.06-.06 2.12 2.12-.06.06A1.7 1.7 0 0 0 17.4 7.7 1.7 1.7 0 0 0 18.96 9H21v3h-2.04A1.7 1.7 0 0 0 17.4 15z" />
      </>
    ),
    chevronDown: <path d="m6 9 6 6 6-6" />,
    chevronRight: <path d="m9 18 6-6-6-6" />,
  };
  return (
    <svg className="icon" viewBox="0 0 24 24" aria-hidden="true">
      {paths[name]}
    </svg>
  );
}

export function Shell({
  active,
  children,
}: {
  active: "sessions" | "diagnostics" | "settings";
  children: ReactNode;
}) {
  return (
    <div className="shell">
      <aside className="sidebar">
        <Logo />
        <nav className="nav">
          <NavLink to="/sessions" active={active === "sessions"} icon="activity">
            Sessions
          </NavLink>
          <NavLink to="/diagnostics" active={active === "diagnostics"} icon="diagnostics">
            Diagnostics
          </NavLink>
          <a href="/readyz">
            <span>
              <Icon name="server" />
            </span>
            System
          </a>
          <NavLink to="/settings" active={active === "settings"} icon="settings">
            Settings
          </NavLink>
        </nav>
        <div className="side-bottom">
          <div className="health">
            <span className="live-dot" /> <strong>System healthy</strong>
            <br />
            <span className="muted">All services operational</span>
            <b>
              <Icon name="chevronRight" />
            </b>
          </div>
          <div className="operator">
            <span>OP</span>
            <p>
              operator
              <br />
              <small>Administrator</small>
            </p>
            <b>
              <Icon name="chevronDown" />
            </b>
          </div>
          <footer>
            remote-tape v1.6.2
            <br />
            View changelog ↗
          </footer>
        </div>
      </aside>
      <main className="main">{children}</main>
    </div>
  );
}

function NavLink({
  to,
  active,
  icon,
  children,
}: {
  to: string;
  active: boolean;
  icon: NavIcon;
  children: ReactNode;
}) {
  return (
    <Link className={active ? "active" : ""} to={to}>
      <span>
        <Icon name={icon} />
      </span>
      {children}
    </Link>
  );
}

export function Logo({ large = false, centered = false }: { large?: boolean; centered?: boolean }) {
  return (
    <div className={`logo ${large ? "large" : ""} ${centered ? "centered" : ""}`}>
      <span className="logo-mark" aria-hidden="true">
        <span />
        <span />
      </span>
      <div>
        <strong>remote-tape</strong>
        <small>remote podcast recorder</small>
      </div>
    </div>
  );
}
