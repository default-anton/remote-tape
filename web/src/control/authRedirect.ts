export function redirectToLogin() {
  if (window.location.pathname !== "/login") window.location.assign("/login");
}

export function shouldRedirectUnauthorized(path: string, status: number) {
  if (status !== 401) return false;
  if (path === "/api/auth/session" || path.startsWith("/api/join/")) return false;
  return true;
}
