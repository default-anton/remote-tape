export function messageFromError(error: unknown) {
  return error instanceof Error ? error.message : "Request failed";
}
