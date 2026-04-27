import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationOptions,
} from "@tanstack/react-query";
import { createSession, getSession, joinSession, listSessions } from "./client";
import { shouldPollJoin, shouldPollSession } from "../domain/sessionStatus";
import type {
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
