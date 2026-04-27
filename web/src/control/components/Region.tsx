export function Region({ region }: { region: string }) {
  return (
    <span className="region">
      {region.startsWith("eu") ? "🇪🇺" : region.startsWith("ap") ? "🇸🇬" : "🇺🇸"} {regionLabel(region)}
    </span>
  );
}

export function regionLabel(region: string) {
  return (
    (
      {
        nyc3: "us-east-1",
        "us-east-1": "us-east-1",
        "us-west-2": "us-west-2",
        "eu-central-1": "eu-central-1",
        "eu-west-1": "eu-west-1",
        "ap-southeast-1": "ap-southeast-1",
      } as Record<string, string>
    )[region] ?? region
  );
}
