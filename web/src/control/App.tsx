import { useEffect, type ReactNode } from "react";
import { Navigate, Route, Routes } from "react-router";
import { useAuthSession } from "./api/hooks";
import { redirectToLogin } from "./authRedirect";
import { JoinPage } from "./routes/JoinPage";
import { LoginPage } from "./routes/LoginPage";
import { NotFound } from "./routes/NotFound";
import { PlaceholderPage } from "./routes/PlaceholderPage";
import { SessionDetailPage } from "./routes/SessionDetailPage";
import { CreateSessionPage, SessionsPage } from "./routes/SessionPage";

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/join" element={<JoinPage />} />
      <Route path="/join/:slug" element={<JoinPage />} />
      <Route path="/join/:slug/*" element={<JoinPage />} />
      <Route
        path="/*"
        element={
          <RequireAuth>
            <ProtectedControlRoutes />
          </RequireAuth>
        }
      />
    </Routes>
  );
}

function ProtectedControlRoutes() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/sessions" replace />} />
      <Route path="/sessions" element={<SessionsPage />} />
      <Route path="/sessions/new" element={<CreateSessionPage />} />
      <Route path="/sessions/:id" element={<SessionDetailPage />} />
      <Route path="/diagnostics" element={<PlaceholderPage title="Diagnostics" />} />
      <Route path="/settings" element={<PlaceholderPage title="Settings" />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}

function RequireAuth({ children }: { children: ReactNode }) {
  const auth = useAuthSession();
  const denied = auth.data?.authenticated === false;

  useEffect(() => {
    if (!auth.isLoading && denied) redirectToLogin();
  }, [auth.isLoading, denied]);

  if (auth.isLoading) return <p className="auth-loading">Checking session…</p>;
  if (auth.isError) return <p className="auth-loading">Unable to check session.</p>;
  if (denied) return null;
  return children;
}
