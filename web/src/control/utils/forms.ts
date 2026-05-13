export function blankAsUndefined(input: string) {
  const trimmed = input.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

export function slugify(input: string) {
  let slug = "";
  let lastDash = false;

  for (const char of input.trim().toLowerCase()) {
    if (/[a-z0-9]/.test(char)) {
      slug += char;
      lastDash = false;
    } else if (!lastDash && slug.length > 0) {
      slug += "-";
      lastDash = true;
    }
    if (slug.length >= 63) break;
  }

  slug = slug.replace(/^-+|-+$/g, "");
  return slug || "session";
}
