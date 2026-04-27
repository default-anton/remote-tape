import { Navigate, Route, Routes } from "react-router";
import { LoginPage } from "./routes/LoginPage";
import { PlaceholderPage } from "./routes/PlaceholderPage";
import { JoinPage } from "./routes/JoinPage";
import { NotFound } from "./routes/NotFound";
import { SessionDetailPage } from "./routes/SessionDetailPage";
import { CreateSessionPage, SessionsPage } from "./routes/SessionPage";

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/sessions" replace />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/sessions" element={<SessionsPage />} />
      <Route path="/sessions/new" element={<CreateSessionPage />} />
      <Route path="/sessions/:id" element={<SessionDetailPage />} />
      <Route path="/join/:slug" element={<JoinPage />} />
      <Route path="/diagnostics" element={<PlaceholderPage title="Diagnostics" />} />
      <Route path="/settings" element={<PlaceholderPage title="Settings" />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
