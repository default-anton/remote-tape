import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { App } from "../App";
import { AuthApp } from "../../auth/App";
import { JoinApp } from "../../join/App";

export function renderApp(path = "/sessions") {
  window.history.pushState({}, "", path);
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  const RoutedApp = path.startsWith("/login") ? AuthApp : path.startsWith("/join") ? JoinApp : App;

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <RoutedApp />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
