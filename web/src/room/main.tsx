import React from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

function RoomApp() {
  return (
    <main className="room-shell">
      <section className="room-card">
        <div className="logo">remote-tape</div>
        <p className="tagline">room runtime placeholder</p>
        <h1>Room UI</h1>
        <p>
          The room bundle is intentionally separate from the control UI. LiveKit, local recording,
          chunk upload, and recovery flows land in the session-runtime slices.
        </p>
      </section>
    </main>
  );
}

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing root element");
}

createRoot(root).render(
  <React.StrictMode>
    <RoomApp />
  </React.StrictMode>,
);
