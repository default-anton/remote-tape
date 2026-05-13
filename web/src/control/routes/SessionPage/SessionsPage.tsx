import { useMemo } from "react";
import { Link, useLocation, useSearchParams } from "react-router";
import { useSessions } from "../../api/hooks";
import { Alert } from "../../components/Alert";
import { Shell } from "../../components/Shell";
import { messageFromError } from "../../utils/errors";
import { SessionTable, type SessionSort } from "./components/SessionTable";
import { Stats } from "./components/Stats";
import { Toolbar } from "./components/Toolbar";

const defaultPage = 1;
const defaultPageSize = 10;
const defaultSort: SessionSort = "updated_at";
const defaultDirection = "desc";

export function SessionsPage() {
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const tableState = useMemo(() => tableStateFromParams(searchParams), [searchParams]);
  const sessions = useSessions({
    page: tableState.page,
    pageSize: tableState.pageSize,
    sort: tableState.sort,
    direction: tableState.direction,
    statuses: tableState.statuses,
    regions: tableState.regions,
    query: tableState.query,
  });
  const rows = sessions.data?.sessions ?? [];

  function updateTable(next: Partial<TableState>) {
    setSearchParams(paramsFromTableState({ ...tableState, ...next }, searchParams));
  }

  function setFilter(next: Partial<Pick<TableState, "query" | "regions" | "statuses">>) {
    updateTable({ ...next, page: 1 });
  }

  function setSort(sort: SessionSort) {
    const direction = tableState.sort === sort && tableState.direction === "asc" ? "desc" : "asc";
    updateTable({ sort, direction, page: 1 });
  }

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
      <Stats summary={sessions.data?.summary} />
      <section className="panel table-panel">
        <Toolbar
          filters={sessions.data?.filters}
          query={tableState.query}
          regions={tableState.regions}
          statuses={tableState.statuses}
          onChange={setFilter}
        />
        {sessions.isLoading ? (
          <p className="muted pad">Loading sessions…</p>
        ) : (
          <SessionTable
            direction={tableState.direction}
            emptyMessage={
              hasActiveFilters(tableState)
                ? "No sessions match the current filters."
                : "No sessions yet. Create one to get host and guest join links."
            }
            onPageChange={(page) => updateTable({ page })}
            onPageSizeChange={(pageSize) => updateTable({ pageSize, page: 1 })}
            onSort={setSort}
            page={sessions.data?.pagination.page ?? tableState.page}
            pageSize={sessions.data?.pagination.page_size ?? tableState.pageSize}
            sessions={rows}
            sort={tableState.sort}
            total={sessions.data?.pagination.total ?? 0}
            totalPages={sessions.data?.pagination.total_pages ?? 0}
          />
        )}
      </section>
    </Shell>
  );
}

type TableState = {
  page: number;
  pageSize: number;
  sort: SessionSort;
  direction: "asc" | "desc";
  statuses: string[];
  regions: string[];
  query: string;
};

function tableStateFromParams(params: URLSearchParams): TableState {
  return {
    page: positiveInt(params.get("page"), defaultPage),
    pageSize: positiveInt(params.get("page_size"), defaultPageSize),
    sort: sessionSort(params.get("sort")),
    direction: params.get("direction") === "asc" ? "asc" : defaultDirection,
    statuses: normalizeFilterValues(params.getAll("status")),
    regions: normalizeFilterValues(params.getAll("region")),
    query: params.get("q") ?? "",
  };
}

function paramsFromTableState(state: TableState, current: URLSearchParams) {
  const next = new URLSearchParams(current);
  setParam(next, "page", state.page === defaultPage ? "" : String(state.page));
  setParam(next, "page_size", state.pageSize === defaultPageSize ? "" : String(state.pageSize));
  setParam(next, "sort", state.sort === defaultSort ? "" : state.sort);
  setParam(next, "direction", state.direction === defaultDirection ? "" : state.direction);
  setParams(next, "status", state.statuses);
  setParams(next, "region", state.regions);
  setParam(next, "q", state.query.trim());
  return next;
}

function positiveInt(value: string | null, fallback: number) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function sessionSort(value: string | null): SessionSort {
  switch (value) {
    case "title":
    case "status":
    case "region":
    case "room_domain":
    case "created_at":
    case "updated_at":
      return value;
    default:
      return defaultSort;
  }
}

function hasActiveFilters(state: TableState) {
  return Boolean(state.query.trim() || state.statuses.length > 0 || state.regions.length > 0);
}

function setParam(params: URLSearchParams, key: string, value: string) {
  if (value) {
    params.set(key, value);
  } else {
    params.delete(key);
  }
}

function setParams(params: URLSearchParams, key: string, values: string[]) {
  params.delete(key);
  for (const value of normalizeFilterValues(values)) {
    params.append(key, value);
  }
}

function normalizeFilterValues(values: string[]) {
  const seen = new Set<string>();
  const normalized: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed || seen.has(trimmed)) continue;
    seen.add(trimmed);
    normalized.push(trimmed);
  }
  return normalized;
}
