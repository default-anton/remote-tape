import { type ReactNode } from "react";
import { Link } from "react-router";

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
          <NavLink to="/sessions" active={active === "sessions"} icon="⌁">
            Sessions
          </NavLink>
          <NavLink to="/diagnostics" active={active === "diagnostics"} icon="♧">
            Diagnostics
          </NavLink>
          <a href="/readyz">
            <span>▤</span>System
          </a>
          <NavLink to="/settings" active={active === "settings"} icon="⚙">
            Settings
          </NavLink>
        </nav>
        <div className="side-bottom">
          <div className="health">
            <span className="live-dot" /> <strong>System healthy</strong>
            <br />
            <span className="muted">All services operational</span>
            <b>›</b>
          </div>
          <div className="operator">
            <span>OP</span>
            <p>
              operator
              <br />
              <small>Administrator</small>
            </p>
            <b>⌄</b>
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
  icon: string;
  children: ReactNode;
}) {
  return (
    <Link className={active ? "active" : ""} to={to}>
      <span>{icon}</span>
      {children}
    </Link>
  );
}

export function Logo({ large = false, centered = false }: { large?: boolean; centered?: boolean }) {
  return (
    <div className={`logo ${large ? "large" : ""} ${centered ? "centered" : ""}`}>
      <span className="logo-mark">●</span>
      <div>
        <strong>remote-tape</strong>
        <small>remote podcast recorder</small>
      </div>
    </div>
  );
}
