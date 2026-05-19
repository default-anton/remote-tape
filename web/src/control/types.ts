import { z } from "zod";
import { SessionStatusSchema } from "./domain/sessionStatus";

const nullableString = z.string().nullable();

export const SessionSchema = z.object({
  id: z.string(),
  slug: z.string(),
  title: z.string(),
  status: SessionStatusSchema,
  instance_id: nullableString,
  public_ip: nullableString,
  instance_region: z.string(),
  instance_size: z.string(),
  image_id: z.string(),
  room_domain: nullableString,
  dns_record_id: nullableString,
  livekit_url: nullableString,
  recording_download_url: nullableString,
  finalization_summary_json: nullableString,
  created_at: z.string(),
  updated_at: z.string(),
  ready_at: nullableString,
  active_at: nullableString,
  finalization_started_at: nullableString,
  finalized_at: nullableString,
  last_heartbeat_at: nullableString,
  download_confirmed_at: nullableString,
  download_confirmed_by: nullableString,
  ended_at: nullableString,
  expires_at: nullableString,
  last_error: nullableString,
  last_error_at: nullableString,
  last_error_phase: nullableString,
  provision_attempts: z.number(),
  dns_attempts: z.number(),
  health_attempts: z.number(),
  teardown_attempts: z.number(),
});

export const AccessTokenSchema = z.object({
  id: z.string(),
  session_id: z.string(),
  role: z.enum(["host", "guest"]),
  label: nullableString,
  created_at: z.string(),
  last_used_at: nullableString,
  revoked_at: nullableString,
});

export const EventSchema = z.object({
  id: z.number(),
  session_id: z.string(),
  type: z.string(),
  message: nullableString,
  metadata_json: nullableString,
  created_at: z.string(),
});

export const DetailSchema = z.object({
  session: SessionSchema,
  access_tokens: z.array(AccessTokenSchema),
  events: z.array(EventSchema),
});

export const SessionEventsResponseSchema = z.object({
  events: z.array(EventSchema),
  pagination: z.object({
    page: z.number(),
    page_size: z.number(),
    total: z.number(),
    total_pages: z.number(),
  }),
});

export const ProvisioningOptionsSchema = z.object({
  defaults: z.object({
    region: z.string(),
    size: z.string(),
  }),
  regions: z.array(
    z.object({
      slug: z.string(),
      label: z.string(),
    }),
  ),
  sizes: z.array(
    z.object({
      slug: z.string(),
      label: z.string(),
      description: z.string(),
      recommended: z.boolean(),
      dedicated_cpu: z.boolean(),
    }),
  ),
  availability: z.record(z.string(), z.array(z.string())),
  recommended_size_by_region: z.record(z.string(), z.string()),
});

export const SessionFilterOptionSchema = z.object({
  value: z.string(),
  label: z.string(),
});

export const SessionsResponseSchema = z.object({
  sessions: z.array(SessionSchema),
  pagination: z.object({
    page: z.number(),
    page_size: z.number(),
    total: z.number(),
    total_pages: z.number(),
  }),
  summary: z.object({
    total: z.number(),
    provisioning: z.number(),
    ready: z.number(),
    active: z.number(),
    awaiting_manual_download: z.number(),
    failed: z.number(),
  }),
  filters: z.object({
    statuses: z.array(SessionFilterOptionSchema),
    regions: z.array(SessionFilterOptionSchema),
  }),
  has_pollable: z.boolean(),
  provisioning_options: ProvisioningOptionsSchema,
});

export const SlugAvailabilitySchema = z.object({
  slug: z.string(),
  normalized_slug: z.string(),
  available: z.boolean(),
  valid: z.boolean(),
  reason: z.enum(["taken", "invalid_format"]).nullable(),
});

export const JoinLinkSchema = z.object({
  url: z.string(),
  role: z.enum(["host", "guest"]),
});

export const TokenInfoSchema = z.object({
  id: z.string(),
  token: z.string(),
});

export const CreateSessionResponseSchema = z.object({
  session: SessionSchema,
  join_links: z.object({
    host: JoinLinkSchema,
    guest: JoinLinkSchema,
  }),
  events: z.array(EventSchema),
  tokens: z.object({
    host: TokenInfoSchema,
    guest: TokenInfoSchema,
  }),
});

export const AuthSessionSchema = z.object({
  authenticated: z.boolean(),
  subject: z.string(),
  csrf_token: z.string(),
});

export const JoinResponseSchema = z.object({
  session: z.object({
    slug: z.string(),
    title: z.string(),
    status: SessionStatusSchema,
  }),
  token: z.object({
    role: z.enum(["host", "guest"]),
  }),
});

export type Session = z.infer<typeof SessionSchema>;
export type SessionFilterOption = z.infer<typeof SessionFilterOptionSchema>;
export type SessionsResponse = z.infer<typeof SessionsResponseSchema>;
export type ProvisioningOptions = z.infer<typeof ProvisioningOptionsSchema>;
export type SlugAvailability = z.infer<typeof SlugAvailabilitySchema>;
export type AccessToken = z.infer<typeof AccessTokenSchema>;
export type Event = z.infer<typeof EventSchema>;
export type Detail = z.infer<typeof DetailSchema>;
export type SessionEventsResponse = z.infer<typeof SessionEventsResponseSchema>;
export type CreateSessionResponse = z.infer<typeof CreateSessionResponseSchema>;
export type AuthSession = z.infer<typeof AuthSessionSchema>;
export type JoinResponse = z.infer<typeof JoinResponseSchema>;

export type CreateSessionInput = {
  title: string;
  slug?: string;
  instance_region?: string;
  instance_size?: string;
};
