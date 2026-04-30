/// <reference types="vitest" />
import type { IncomingMessage } from "node:http";
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";

function controlHistoryFallback(): Plugin {
  return {
    name: "remote-tape-control-history-fallback",
    configureServer(server) {
      server.middlewares.use((req, _res, next) => {
        if (req.method !== "GET" && req.method !== "HEAD") {
          next();
          return;
        }
        const url = new URL(req.url ?? "/", "http://127.0.0.1");
        const path = url.pathname;
        const isControlAPI = path === "/api" || path.startsWith("/api/");
        const isProxiedStatus = path === "/healthz" || path === "/readyz";
        const isStaticAsset =
          path.startsWith("/@") ||
          path.startsWith("/assets/") ||
          path.startsWith("/src/") ||
          path.startsWith("/favicon") ||
          path.includes(".");
        if (!isControlAPI && !isProxiedStatus && !isStaticAsset) {
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
  const outDir = target === "control" ? "../internal/controlui/dist/control" : "dist/room";
  const proxy =
    target === "control" && mode !== "mock"
      ? {
          "/api": "http://127.0.0.1:8080",
          "/healthz": "http://127.0.0.1:8080",
          "/readyz": "http://127.0.0.1:8080",
          "/login": {
            target: "http://127.0.0.1:8080",
            bypass: (req: IncomingMessage) => (req.method === "POST" ? undefined : req.url),
          },
          "/logout": {
            target: "http://127.0.0.1:8080",
            bypass: (req: IncomingMessage) => (req.method === "POST" ? undefined : req.url),
          },
        }
      : undefined;

  return {
    base: "/",
    plugins: target === "control" ? [controlHistoryFallback(), react()] : [react()],
    publicDir: mode === "mock" ? "public" : false,
    build: {
      outDir,
      emptyOutDir: true,
      rollupOptions: {
        input: {
          [target]: resolve(__dirname, htmlFile),
        },
      },
    },
    server: {
      port: target === "room" ? 5174 : 5173,
      proxy,
    },
    test: {
      environment: "jsdom",
      setupFiles: "./src/test/setup.ts",
      globals: true,
    },
  };
});
