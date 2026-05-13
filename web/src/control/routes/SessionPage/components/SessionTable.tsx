import { Link, useLocation } from "react-router";
import { Icon } from "../../../components/Icon";
import { Region } from "../../../components/Region";
import { StatusBadge } from "../../../components/StatusBadge";
import { sessionStatusTone } from "../../../domain/sessionStatus";
import type { Session } from "../../../types";
import { formatDateTime } from "../../../utils/format";
import { domainFor } from "../../../utils/session";
import { actionIcon } from "./sessionActions";

export function SessionTable({ sessions }: { sessions: Session[] }) {
  const location = useLocation();
  if (sessions.length === 0)
    return (
      <p className="muted pad">No sessions yet. Create one to get host and guest join links.</p>
    );

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Session ↕</th>
            <th>Status</th>
            <th>Region</th>
            <th>Room domain</th>
            <th>Created ↕</th>
            <th>Updated ↕</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {sessions.slice(0, 10).map((session) => {
            const roomDomain = domainFor(session);
            return (
              <tr key={session.id}>
                <td>
                  <span className={`tiny-dot ${sessionStatusTone(session.status)}`} />
                  <Link
                    className="row-title"
                    to={{ pathname: `/sessions/${session.id}`, search: location.search }}
                  >
                    {session.title}
                  </Link>
                  <br />
                  <span className="row-id">{session.id}</span>
                </td>
                <td>
                  <StatusBadge status={session.status} />
                </td>
                <td>
                  <Region region={session.instance_region} />
                </td>
                <td>
                  <span className="domain-cell" title={roomDomain}>
                    {roomDomain}
                  </span>
                </td>
                <td>{formatDateTime(session.created_at)}</td>
                <td>
                  {formatDateTime(session.updated_at)}{" "}
                  {session.status === "active" ? <span className="live-dot" /> : null}
                </td>
                <td>
                  <div className="actions">
                    <button className="button icon-only" type="button">
                      <Icon name={actionIcon(session.status)} />
                    </button>
                    <button className="button icon-only" type="button">
                      <Icon name="more" />
                    </button>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      <div className="pager">
        <span>
          Showing 1–{Math.min(10, sessions.length)} of {Math.max(24, sessions.length)} sessions
        </span>
        <div>
          <button className="button ghost icon-only" type="button">
            <Icon name="chevronLeft" />
          </button>
          <button className="button ghost current" type="button">
            1
          </button>
          <button className="button ghost" type="button">
            2
          </button>
          <button className="button ghost" type="button">
            3
          </button>
          <button className="button ghost icon-only" type="button">
            <Icon name="chevronRight" />
          </button>
        </div>
        <button className="button ghost" type="button">
          10 per page <Icon name="chevronDown" />
        </button>
      </div>
    </div>
  );
}
