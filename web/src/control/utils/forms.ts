export function blankAsUndefined(input: string) {
  const trimmed = input.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

export function slugify(input: string) {
  return input
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 63);
}
