export function Region({ region }: { region: string }) {
  return <span className="region">🌐 {regionLabel(region)}</span>;
}

export function regionLabel(region: string) {
  return region;
}
