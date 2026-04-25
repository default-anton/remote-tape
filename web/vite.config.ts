/// <reference types="vitest" />
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";

function controlHistoryFallback(): Plugin {
  return {
    name: "remote-tape-control-history-fallback",
    configureServer(server) {
      server.middlewares.use((req, _res, next) => {
        const url = req.url ?? "/";
        if (url === "/" || url.startsWith("/sessions") || url.startsWith("/join/")) {
          req.url = "/index.control.html";
        }
        next();
      });
    },
  };
}

export default defineConfig(({ mode }) => {
  const target = mode === "room" ? "room" : "control";
  const htmlFile = target === "room" ? "index.room.html" : "index.control.html";

  return {
    plugins: target === "control" ? [controlHistoryFallback(), react()] : [react()],
    build: {
      outDir: `dist/${target}`,
      emptyOutDir: true,
      rollupOptions: {
        input: {
          [target]: resolve(__dirname, htmlFile),
        },
      },
    },
    server: {
      port: target === "room" ? 5174 : 5173,
      proxy:
        target === "control"
          ? {
              "/api": "http://127.0.0.1:8080",
              "/healthz": "http://127.0.0.1:8080",
              "/readyz": "http://127.0.0.1:8080",
            }
          : undefined,
    },
    test: {
      environment: "jsdom",
      setupFiles: "./src/test/setup.ts",
      globals: true,
    },
  };
});
