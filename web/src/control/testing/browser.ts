import { setupWorker } from "msw/browser";
import { createControlMockApi } from "./handlers";

export async function startControlMocks() {
  const api = createControlMockApi();
  const worker = setupWorker(...api.handlers);
  await worker.start({
    onUnhandledRequest(request, print) {
      const url = new URL(request.url);
      if (url.pathname === "/api" || url.pathname.startsWith("/api/")) {
        print.error();
      }
    },
  });
}
