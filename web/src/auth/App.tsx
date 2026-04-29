import { Route, Routes } from "react-router";
import { LoginPage } from "../control/routes/LoginPage";

export function AuthApp() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="*" element={<LoginPage />} />
    </Routes>
  );
}
