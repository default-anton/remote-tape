import { Route, Routes } from "react-router";
import { JoinPage } from "../control/routes/JoinPage";

export function JoinApp() {
  return (
    <Routes>
      <Route path="/join/:slug" element={<JoinPage />} />
      <Route path="*" element={<JoinPage />} />
    </Routes>
  );
}
