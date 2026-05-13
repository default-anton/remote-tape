import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Link, useLocation } from "react-router";
import { Alert } from "../../../components/Alert";
import { Icon } from "../../../components/Icon";
import type { CreateSessionInput, ProvisioningOptions } from "../../../types";
import { messageFromError } from "../../../utils/errors";
import { blankAsUndefined, slugify } from "../../../utils/forms";
import { Field } from "./Field";
import type { ProvisioningSelection } from "./ProvisionCard";

export function CreateSessionForm({
  busy,
  error,
  options,
  optionsError,
  optionsLoading,
  onProvisioningSelectionChange,
  onSubmit,
}: {
  busy: boolean;
  error: Error | null;
  options: ProvisioningOptions | undefined;
  optionsError: Error | null;
  optionsLoading: boolean;
  onProvisioningSelectionChange: (selection: ProvisioningSelection) => void;
  onSubmit: (input: CreateSessionInput) => void;
}) {
  const location = useLocation();
  const [title, setTitle] = useState("The Infra Podcast #313");
  const [slug, setSlug] = useState("the-infra-podcast-313");
  const [slugDirty, setSlugDirty] = useState(false);
  const [region, setRegion] = useState("");
  const [size, setSize] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const appliedDefaults = useRef(false);

  useEffect(() => {
    if (!options || appliedDefaults.current) return;
    setRegion(options.defaults.region);
    setSize(options.defaults.size);
    appliedDefaults.current = true;
  }, [options]);

  useEffect(() => {
    onProvisioningSelectionChange({ region, size });
  }, [onProvisioningSelectionChange, region, size]);

  const availableSizeSlugs = useMemo(() => {
    if (!options) return new Set<string>();
    return new Set(options.availability[region] ?? []);
  }, [options, region]);

  const availableSizes = useMemo(() => {
    if (!options) return [];
    return options.sizes.filter((candidate) => availableSizeSlugs.has(candidate.slug));
  }, [availableSizeSlugs, options]);

  function updateRegion(nextRegion: string) {
    setRegion(nextRegion);
    setValidationError(null);
    if (!options || !isSupportedRegion(options, nextRegion)) return;

    const nextAvailableSizes = options.availability[nextRegion] ?? [];
    if (size !== "" && nextAvailableSizes.includes(size)) return;

    const recommended = options.recommended_size_by_region[nextRegion];
    if (recommended && nextAvailableSizes.includes(recommended)) {
      setSize(recommended);
      return;
    }
    setSize("");
    setValidationError("Choose an instance size available in the selected region.");
  }

  function updateSize(nextSize: string) {
    setSize(nextSize);
    setValidationError(null);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!options) return;
    const error = validateProvisioningInput(options, region, size);
    if (error) {
      setValidationError(error);
      return;
    }
    onSubmit({
      title,
      slug: blankAsUndefined(slug),
      instance_region: blankAsUndefined(region),
      instance_size: blankAsUndefined(size),
    });
  }

  const disabled = busy || optionsLoading || Boolean(optionsError) || !options;

  return (
    <form className="create-form" onSubmit={submit}>
      <section className="panel form-panel">
        {error ? <Alert>{messageFromError(error)}</Alert> : null}
        {optionsError ? (
          <Alert>Provisioning options failed to load: {messageFromError(optionsError)}</Alert>
        ) : null}
        {optionsLoading ? <Alert>Loading provisioning options…</Alert> : null}
        <Field
          label="Session title"
          help="A friendly name to identify your session."
          count={`${title.length} / 100`}
        >
          <input
            id="title"
            value={title}
            onChange={(event) => {
              setTitle(event.target.value);
              if (!slugDirty) setSlug(slugify(event.target.value));
            }}
            required
            maxLength={100}
            placeholder="The Infra Podcast #313"
          />
        </Field>
        <Field
          label="Session slug"
          help="Used for the session server domain. Lowercase letters, numbers, and dashes only."
          count={`${slug.length} / 63`}
        >
          <input
            id="slug"
            value={slug}
            onChange={(event) => {
              setSlugDirty(true);
              setSlug(event.target.value);
            }}
            pattern="[a-z0-9-]{1,63}"
            placeholder="the-infra-podcast-313"
          />
          <div className="domain-preview" aria-live="polite">
            <span>Session server domain</span>
            <strong>{slug || "session-slug"}.remote-tape.io</strong>
          </div>
          <div className="ok-line">✓ Slug is available</div>
        </Field>
        <Field
          label="Preferred region"
          help="Select the region closest to your guests for the best recording quality."
        >
          <div className="selectish">
            <select
              id="region"
              value={region}
              onChange={(event) => updateRegion(event.target.value)}
              disabled={!options || optionsLoading}
            >
              <option value="" disabled>
                {optionsLoading ? "Loading regions…" : "Use backend default"}
              </option>
              {options?.regions.map((option) => (
                <option key={option.slug} value={option.slug}>
                  {option.label} ({option.slug})
                </option>
              ))}
            </select>
            <Icon name="chevronDown" />
          </div>
        </Field>
        <Field
          label="Instance size"
          help="Larger instances provide more headroom for high-bitrate recordings."
        >
          <div className="input-wrap selectish">
            <select
              id="size"
              value={size}
              onChange={(event) => updateSize(event.target.value)}
              disabled={!options || optionsLoading || availableSizes.length === 0}
            >
              <option value="" disabled>
                {optionsLoading ? "Loading sizes…" : "Use backend default"}
              </option>
              {availableSizes.map((option) => (
                <option key={option.slug} value={option.slug}>
                  {option.slug} — {option.label}, {option.description}
                </option>
              ))}
            </select>
            <Icon name="chevronDown" />
          </div>
          {validationError ? <div className="field-error">{validationError}</div> : null}
        </Field>
        <Field
          label="Notes (optional)"
          help="Any additional context about this recording."
          count="0 / 500"
        >
          <textarea placeholder="e.g. episode topic, guests, recording plan…" />
        </Field>
        <button type="button" className="button ghost advanced">
          Advanced options <Icon name="chevronRight" />
          <small>Tags, data retention, recording settings, and more.</small>
        </button>
      </section>
      <div className="form-actions">
        <Link className="button ghost" to={{ pathname: "/sessions", search: location.search }}>
          Cancel
        </Link>
        <button
          aria-busy={busy}
          aria-label="+ Create session"
          className="button primary"
          type="submit"
          disabled={disabled}
        >
          {busy ? (
            "Creating…"
          ) : (
            <>
              <span className="plus">＋</span> Create session
            </>
          )}
        </button>
      </div>
    </form>
  );
}

function validateProvisioningInput(options: ProvisioningOptions, region: string, size: string) {
  if (!isSupportedRegion(options, region)) {
    return `Unsupported region "${region}". Choose one of the suggested region slugs.`;
  }
  if (!options.sizes.some((option) => option.slug === size)) {
    return `Unsupported instance size "${size}". Choose one of the suggested size slugs.`;
  }
  if (!options.availability[region]?.includes(size)) {
    return `Instance size "${size}" is not available in region "${region}".`;
  }
  return null;
}

function isSupportedRegion(options: ProvisioningOptions, region: string) {
  return options.regions.some((option) => option.slug === region);
}
