import { Link, useLocation } from "react-router";
import { Icon } from "../../../components/Icon";
import { Region } from "../../../components/Region";
import { StatusBadge } from "../../../components/StatusBadge";
import { sessionStatusTone } from "../../../domain/sessionStatus";
import type { Session } from "../../../types";
import { formatDateTime } from "../../../utils/format";
import { domainFor } from "../../../utils/session";

export type SessionSort =
  | "title"
  | "status"
  | "region"
  | "room_domain"
  | "created_at"
  | "updated_at";

const pageSizes = [10, 25, 50, 100];

export function SessionTable({
  direction,
  emptyMessage,
  onPageChange,
  onPageSizeChange,
  onSort,
  page,
  pageSize,
  sessions,
  sort,
  total,
  totalPages,
}: {
  direction: "asc" | "desc";
  emptyMessage: string;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  onSort: (sort: SessionSort) => void;
  page: number;
  pageSize: number;
  sessions: Session[];
  sort: SessionSort;
  total: number;
  totalPages: number;
}) {
  const location = useLocation();
  if (sessions.length === 0) {
    return <p className="muted pad">{emptyMessage}</p>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <SortHeader
              active={sort}
              direction={direction}
              label="Session"
              sort="title"
              onSort={onSort}
            />
            <SortHeader
              active={sort}
              direction={direction}
              label="Status"
              sort="status"
              onSort={onSort}
            />
            <SortHeader
              active={sort}
              direction={direction}
              label="Region"
              sort="region"
              onSort={onSort}
            />
            <SortHeader
              active={sort}
              direction={direction}
              label="Room domain"
              sort="room_domain"
              onSort={onSort}
            />
            <SortHeader
              active={sort}
              direction={direction}
              label="Created"
              sort="created_at"
              onSort={onSort}
            />
            <SortHeader
              active={sort}
              direction={direction}
              label="Updated"
              sort="updated_at"
              onSort={onSort}
            />
          </tr>
        </thead>
        <tbody>
          {sessions.map((session) => {
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
              </tr>
            );
          })}
        </tbody>
      </table>
      <Pager
        page={page}
        pageSize={pageSize}
        total={total}
        totalPages={totalPages}
        onPageChange={onPageChange}
        onPageSizeChange={onPageSizeChange}
      />
    </div>
  );
}

function SortHeader({
  active,
  direction,
  label,
  sort,
  onSort,
}: {
  active: SessionSort;
  direction: "asc" | "desc";
  label: string;
  sort: SessionSort;
  onSort: (sort: SessionSort) => void;
}) {
  const selected = active === sort;
  return (
    <th>
      <button
        aria-label={`Sort by ${label}`}
        aria-sort={selected ? (direction === "asc" ? "ascending" : "descending") : undefined}
        className="sort-header"
        type="button"
        onClick={() => onSort(sort)}
      >
        {label} <span>{selected ? (direction === "asc" ? "↑" : "↓") : "↕"}</span>
      </button>
    </th>
  );
}

function Pager({
  page,
  pageSize,
  total,
  totalPages,
  onPageChange,
  onPageSizeChange,
}: {
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
}) {
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const end = Math.min(total, page * pageSize);
  const pages = pageWindow(page, totalPages);
  return (
    <div className="pager">
      <span>
        Showing {start}–{end} of {total} sessions
      </span>
      <div>
        <button
          aria-label="Previous page"
          className="button ghost icon-only"
          disabled={page <= 1}
          type="button"
          onClick={() => onPageChange(page - 1)}
        >
          <Icon name="chevronLeft" />
        </button>
        {pages.map((item, index) =>
          item === "…" ? (
            <span className="pager-gap" key={`gap-${index}`}>
              …
            </span>
          ) : (
            <button
              className={`button ghost ${item === page ? "current" : ""}`}
              key={item}
              type="button"
              onClick={() => onPageChange(item)}
            >
              {item}
            </button>
          ),
        )}
        <button
          aria-label="Next page"
          className="button ghost icon-only"
          disabled={page >= totalPages}
          type="button"
          onClick={() => onPageChange(page + 1)}
        >
          <Icon name="chevronRight" />
        </button>
      </div>
      <label className="page-size-select">
        <select value={pageSize} onChange={(event) => onPageSizeChange(Number(event.target.value))}>
          {pageSizes.map((size) => (
            <option key={size} value={size}>
              {size} per page
            </option>
          ))}
        </select>
        <Icon name="chevronDown" />
      </label>
    </div>
  );
}

function pageWindow(page: number, totalPages: number): Array<number | "…"> {
  if (totalPages <= 7) return Array.from({ length: totalPages }, (_, index) => index + 1);
  const pages = new Set([1, totalPages, page - 1, page, page + 1]);
  const result: Array<number | "…"> = [];
  let previous = 0;
  for (const candidate of [...pages]
    .filter((item) => item >= 1 && item <= totalPages)
    .sort((a, b) => a - b)) {
    if (previous !== 0 && candidate - previous > 1) result.push("…");
    result.push(candidate);
    previous = candidate;
  }
  return result;
}
