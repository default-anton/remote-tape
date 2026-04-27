import { Link, useLocation } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { listSessions } from "../../api";
import { Alert } from "../../components/Alert";
import { Shell } from "../../components/Shell";
import { messageFromError } from "../../utils/errors";
import { SessionTable } from "./components/SessionTable";
import { Stats } from "./components/Stats";
import { Toolbar } from "./components/Toolbar";

export function SessionsPage() {
  const location = useLocation();
  const sessions = useQuery({ queryKey: ["sessions"], queryFn: listSessions });
  const rows = sessions.data?.sessions ?? [];

  return (
    <Shell active="sessions">
      <div className="page-head">
        <div>
          <h1>Sessions</h1>
          <p className="lead">Manage recording sessions across regions and lifecycle states.</p>
        </div>
        <Link
          className="button primary"
          to={{ pathname: "/sessions/new", search: location.search }}
        >
          <span className="plus">＋</span> New session
        </Link>
      </div>
      {sessions.isError ? <Alert>{messageFromError(sessions.error)}</Alert> : null}
      <Stats sessions={rows} />
      <section className="panel table-panel">
        <Toolbar />
        {sessions.isLoading ? (
          <p className="muted pad">Loading sessions…</p>
        ) : (
          <SessionTable sessions={rows} />
        )}
      </section>
    </Shell>
  );
}
