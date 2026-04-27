export function formatDateTime(input: string) {
  return (
    <>
      <span>{formatDate(input)}</span>
      <br />
      <span>{formatTime(input)}</span>
    </>
  );
}

export function formatDate(input: string) {
  const date = new Date(input);
  return Number.isNaN(date.getTime())
    ? input
    : new Intl.DateTimeFormat("en", { month: "short", day: "numeric", year: "numeric" }).format(
        date,
      );
}

export function formatTime(input: string) {
  const date = new Date(input);
  return Number.isNaN(date.getTime())
    ? ""
    : new Intl.DateTimeFormat("en", { hour: "numeric", minute: "2-digit" }).format(date);
}
