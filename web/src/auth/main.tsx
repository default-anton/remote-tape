import React from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { AuthApp } from "./App";
import { startMockApiIfNeeded } from "../control/testing/mockMode";
import "../control/styles.css";
import "../control/styles-detail.css";

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
  await startMockApiIfNeeded();

  createRoot(container).render(
    <React.StrictMode>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <AuthApp />
        </BrowserRouter>
      </QueryClientProvider>
    </React.StrictMode>,
  );
}

void bootstrap();
