import { Icon } from "../../../components/Icon";
import type { SessionFilterOption } from "../../../types";

type ToolbarFilters = {
  statuses: SessionFilterOption[];
  regions: SessionFilterOption[];
};

export function Toolbar({
  filters,
  query,
  region,
  status,
  onChange,
}: {
  filters: ToolbarFilters | undefined;
  query: string;
  region: string;
  status: string;
  onChange: (next: { query?: string; region?: string; status?: string }) => void;
}) {
  return (
    <div className="toolbar">
      <label className="search-field">
        <Icon name="search" />
        <input
          aria-label="Search sessions"
          placeholder="Search sessions…"
          value={query}
          onChange={(event) => onChange({ query: event.target.value })}
        />
      </label>
      <FilterSelect
        label="Status"
        options={filters?.statuses ?? []}
        placeholder="All statuses"
        value={status}
        onChange={(value) => onChange({ status: value })}
      />
      <FilterSelect
        label="Region"
        options={filters?.regions ?? []}
        placeholder="All regions"
        value={region}
        onChange={(value) => onChange({ region: value })}
      />
    </div>
  );
}

function FilterSelect({
  label,
  options,
  placeholder,
  value,
  onChange,
}: {
  label: string;
  options: SessionFilterOption[];
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="filter-select">
      <span>{label}</span>
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="">{placeholder}</option>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <b>
        <Icon name="chevronDown" />
      </b>
    </label>
  );
}
