import { Icon } from "../../../components/Icon";

export function Toolbar() {
  return (
    <div className="toolbar">
      <label className="search-field">
        <Icon name="search" />
        <input aria-label="Search sessions" placeholder="Search sessions…" />
        <kbd>⌘K</kbd>
      </label>
      <FakeSelect label="Status" value="All statuses" />
      <FakeSelect label="Region" value="All regions" />
      <FakeSelect label="Created" value="Any time" />
      <button type="button" className="button ghost">
        <Icon name="filter" /> Filters
      </button>
      <button type="button" className="button icon-only" aria-label="Refresh">
        <Icon name="refresh" />
      </button>
    </div>
  );
}

function FakeSelect({ label, value }: { label: string; value: string }) {
  return (
    <button type="button" className="button ghost fake-select">
      <span>{label}</span>
      {value}
      <b>
        <Icon name="chevronDown" />
      </b>
    </button>
  );
}
