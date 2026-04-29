import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationOptions,
} from "@tanstack/react-query";
import {
  authSession,
  createSession,
  getSession,
  joinSession,
  listSessions,
  login,
  logout,
} from "./client";
import { shouldPollJoin, shouldPollSession } from "../domain/sessionStatus";
import type {
  AuthSession,
  CreateSessionInput,
  CreateSessionResponse,
  Detail,
  JoinResponse,
  Session,
} from "../types";
import { joinKeys, sessionsKeys } from "./queryKeys";

const sessionPollIntervalMs = 5_000;

type SessionsResponse = { sessions: Session[] };

type CreateSessionOptions = Pick<
  UseMutationOptions<CreateSessionResponse, Error, CreateSessionInput>,
  "onError" | "onSettled" | "onSuccess"
>;

type LoginOptions = Pick<UseMutationOptions<void, Error, string>, "onError" | "onSuccess">;
type LogoutOptions = Pick<UseMutationOptions<void, Error, void>, "onError" | "onSuccess">;

export function useAuthSession() {
  return useQuery<AuthSession>({
    queryKey: ["auth", "session"],
    queryFn: authSession,
    retry: false,
  });
}

export function useLogin(options: LoginOptions = {}) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: login,
    retry: false,
    ...options,
    onSuccess: async (result, variables, onMutateResult, context) => {
      await queryClient.invalidateQueries({ queryKey: ["auth", "session"] });
      await options.onSuccess?.(result, variables, onMutateResult, context);
    },
  });
}

export function useLogout(options: LogoutOptions = {}) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: logout,
    retry: false,
    ...options,
    onSuccess: async (result, variables, onMutateResult, context) => {
      queryClient.clear();
      await options.onSuccess?.(result, variables, onMutateResult, context);
    },
  });
}

export function useSessions() {
  return useQuery({
    queryKey: sessionsKeys.list(),
    queryFn: listSessions,
    refetchInterval: (query) => sessionsListRefetchInterval(query.state.data),
  });
}

export function useSessionDetail(id: string | undefined) {
  return useQuery({
    queryKey: sessionsKeys.detail(id ?? ""),
    queryFn: () => getSession(id ?? ""),
    enabled: Boolean(id),
    refetchInterval: (query) => sessionDetailRefetchInterval(query.state.data),
  });
}

export function useCreateSession(options: CreateSessionOptions = {}) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createSession,
    retry: false,
    ...options,
    onSuccess: async (created, variables, onMutateResult, context) => {
      await queryClient.invalidateQueries({ queryKey: sessionsKeys.list() });
      queryClient.setQueryData(sessionsKeys.detail(created.session.id), {
        session: created.session,
        access_tokens: [],
        events: created.events,
      } satisfies Detail);
      await options.onSuccess?.(created, variables, onMutateResult, context);
    },
  });
}

export function useJoinSession(slug: string | undefined, token: string) {
  return useQuery({
    queryKey: joinKeys.detail(slug ?? "", token),
    queryFn: () => joinSession(slug ?? "", token),
    enabled: Boolean(slug && token),
    retry: false,
    refetchInterval: (query) => joinRefetchInterval(query.state.data),
  });
}

export function sessionsListRefetchInterval(data: SessionsResponse | undefined) {
  if (!data?.sessions.some((session) => shouldPollSession(session.status))) return false;
  return sessionPollIntervalMs;
}

export function sessionDetailRefetchInterval(data: Detail | undefined) {
  if (!data || !shouldPollSession(data.session.status)) return false;
  return sessionPollIntervalMs;
}

export function joinRefetchInterval(data: JoinResponse | undefined) {
  if (!data || !shouldPollJoin(data.session.status)) return false;
  return sessionPollIntervalMs;
}
