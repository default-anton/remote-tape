import React from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { App } from "./App";
import "./styles.css";
import "./styles-detail.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 5_000,
    },
  },
});

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing root element");
}
const container = root;

async function bootstrap() {
  if (import.meta.env.MODE === "mock") {
    const { startControlMocks } = await import("./testing/browser");
    await startControlMocks();
  }

  createRoot(container).render(
    <React.StrictMode>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </QueryClientProvider>
    </React.StrictMode>,
  );
}

void bootstrap();
