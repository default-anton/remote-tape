import { useState, type FormEvent } from "react";
import { Link, useLocation } from "react-router";
import { Alert } from "../../../components/Alert";
import { Icon } from "../../../components/Icon";
import { messageFromError } from "../../../utils/errors";
import { blankAsUndefined, slugify } from "../../../utils/forms";
import { Field } from "./Field";

export function CreateSessionForm({
  busy,
  error,
  onSubmit,
}: {
  busy: boolean;
  error: Error | null;
  onSubmit: (input: {
    title: string;
    slug?: string;
    droplet_region?: string;
    droplet_size?: string;
  }) => void;
}) {
  const location = useLocation();
  const [title, setTitle] = useState("The Infra Podcast #313");
  const [slug, setSlug] = useState("the-infra-podcast-313");
  const [slugDirty, setSlugDirty] = useState(false);
  const [region, setRegion] = useState("US East 1 (New York)");
  const [size, setSize] = useState("2 vCPU / 4 GB RAM");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    onSubmit({
      title,
      slug: blankAsUndefined(slug),
      droplet_region: blankAsUndefined(region),
      droplet_size: blankAsUndefined(size),
    });
  }

  return (
    <form className="create-form" onSubmit={submit}>
      <section className="panel form-panel">
        {error ? <Alert>{messageFromError(error)}</Alert> : null}
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
          help="Unique slug used in URLs and subdomains. Lowercase letters, numbers, and dashes only."
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
          <div className="ok-line">✓ Slug is available</div>
        </Field>
        <Field
          label="Preferred region"
          help="Select the region closest to your guests for the best recording quality."
        >
          <div className="selectish">
            <span>🇺🇸</span>
            <input
              id="region"
              value={region}
              onChange={(event) => setRegion(event.target.value)}
              placeholder="Use backend default"
            />
            <Icon name="chevronDown" />
          </div>
        </Field>
        <Field
          label="Droplet size"
          help="Larger droplets provide more headroom for high-bitrate recordings."
        >
          <div className="input-wrap">
            <input
              id="size"
              value={size}
              onChange={(event) => setSize(event.target.value)}
              placeholder="Use backend default"
            />
            <span>Recommended</span>
          </div>
        </Field>
        <Field label="Room subdomain" help="Your guests will use this link to join the session.">
          <div className="subdomain">
            <span>{slug || "session-slug"}</span>
            <b>.remote-tape.io</b>
            <button type="button" aria-label="Copy room subdomain">
              <Icon name="copy" />
            </button>
          </div>
          <a href={`/join/${slug}`}>https://{slug || "session-slug"}.remote-tape.io/join</a>
        </Field>
        <Field
          label="Host name"
          help="Display name shown to guests in the session."
          count="12 / 80"
        >
          <input value="Andrew Mason" readOnly />
        </Field>
        <Field
          label="Notes (optional)"
          help="Any additional context about this recording."
          count="0 / 500"
        >
          <textarea placeholder="e.g. episode topic, guests, recording plan…" />
        </Field>
        <button type="button" className="advanced">
          Advanced options <Icon name="chevronRight" />
          <small>Tags, data retention, recording settings, and more.</small>
        </button>
      </section>
      <div className="form-actions">
        <Link className="button ghost" to={{ pathname: "/sessions", search: location.search }}>
          Cancel
        </Link>
        <button className="primary" type="submit" disabled={busy} aria-label="+ Create session">
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
