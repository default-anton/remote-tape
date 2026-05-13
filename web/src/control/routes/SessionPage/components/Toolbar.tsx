import { useEffect, useId, useRef, useState } from "react";
import { Icon } from "../../../components/Icon";
import type { SessionFilterOption } from "../../../types";

type ToolbarFilters = {
  statuses: SessionFilterOption[];
  regions: SessionFilterOption[];
};

export function Toolbar({
  filters,
  query,
  regions,
  statuses,
  onChange,
}: {
  filters: ToolbarFilters | undefined;
  query: string;
  regions: string[];
  statuses: string[];
  onChange: (next: { query?: string; regions?: string[]; statuses?: string[] }) => void;
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
      <MultiFilter
        label="Status"
        options={filters?.statuses ?? []}
        placeholder="All statuses"
        selected={statuses}
        onChange={(values) => onChange({ statuses: values })}
      />
      <MultiFilter
        label="Region"
        options={filters?.regions ?? []}
        placeholder="All regions"
        selected={regions}
        onChange={(values) => onChange({ regions: values })}
      />
    </div>
  );
}

function MultiFilter({
  label,
  options,
  placeholder,
  selected,
  onChange,
}: {
  label: string;
  options: SessionFilterOption[];
  placeholder: string;
  selected: string[];
  onChange: (values: string[]) => void;
}) {
  const id = useId();
  const labelId = `${id}-label`;
  const summaryId = `${id}-summary`;
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const selectedSet = new Set(selected);
  const labelByValue = new Map(options.map((option) => [option.value, option.label]));
  const selectedLabels = selected.map((value) => labelByValue.get(value) ?? value);
  const summary = selectedLabels.length > 0 ? selectedLabels.join(", ") : placeholder;

  useEffect(() => {
    if (!open) return;

    function closeOnClick(event: MouseEvent) {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    }

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") setOpen(false);
    }

    document.addEventListener("click", closeOnClick);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("click", closeOnClick);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  function toggleValue(value: string) {
    if (selectedSet.has(value)) {
      onChange(selected.filter((item) => item !== value));
      return;
    }
    onChange([...selected, value]);
  }

  function clearSelected() {
    onChange([]);
  }

  return (
    <div className="multi-filter" ref={rootRef}>
      <button
        aria-controls={id}
        aria-expanded={open}
        aria-haspopup="true"
        aria-labelledby={`${labelId} ${summaryId}`}
        className="multi-filter-button"
        type="button"
        onClick={() => setOpen((current) => !current)}
      >
        <span id={labelId}>{label}</span>
        <strong id={summaryId} title={summary}>
          {summary}
        </strong>
        <b>
          <Icon name="chevronDown" />
        </b>
      </button>
      {open ? (
        <div className="multi-filter-popover" id={id} role="group" aria-label={`${label} filter`}>
          <div className="multi-filter-head">
            <span>{selected.length === 0 ? placeholder : `${selected.length} selected`}</span>
            <button
              className="link-button"
              disabled={selected.length === 0}
              type="button"
              onClick={clearSelected}
            >
              Clear
            </button>
          </div>
          <div className="multi-filter-options">
            {options.map((option) => (
              <label className="multi-filter-option" key={option.value}>
                <input
                  checked={selectedSet.has(option.value)}
                  type="checkbox"
                  onChange={() => toggleValue(option.value)}
                />
                <span>{option.label}</span>
              </label>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
