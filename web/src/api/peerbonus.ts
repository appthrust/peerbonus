// Peer Bonus API (`/api/peerbonus/v1/*`, served by cmd/server from the
// SPA's own origin; contract: api.md). The OIDC edge (Envoy) in front of the
// server attaches the caller's bearer token from the session cookie; the SPA
// never sees or requests one. Lists are `{items: [...]}` (the timeline adds
// `nextCursor`), single resources are direct objects, errors are
// `{error, message}` (409 `insufficient_points` adds `remaining`, `required`).
// `POST /posts` and `POST /posts/{id}/claps` carry an `Idempotency-Key`; a
// resend with the same key answers 201 with the current post.

/**
 * `directoryUrl`: the person's Directory profile, null when Directory's browser origin is unknown. `pending`: never signed in.
 * `avatarUrl`: the profile picture relayed on this origin (`/api/peerbonus/v1/avatars/<personId>?v=…`), null when Directory has none.
 */
export type Member = { id: string; personId: string; displayName: string; email: string; pending: boolean; directoryUrl: string | null; avatarUrl: string | null }
/** A Directory group (`path` is slash-separated, e.g. `sales/team-a`). */
export type RecipientGroup = { id: string; name: string; path: string }
/**
 * A Directory person the caller can send to (`GET /recipients`, `GET /members`); no Member exists for
 * them until they are sent points. `groups`: the ones known from the caller's teams, often empty.
 * `avatarUrl`: as on `Member`.
 */
export type Candidate = { personId: string; displayName: string; email: string; pending: boolean; groups: RecipientGroup[]; avatarUrl: string | null }
/**
 * Everything the recipient picker shows before any typing. `me.groups`: the caller's direct groups.
 * `recent`: people the caller sent to, newest first (≤ 8). `teams`: the caller's direct groups, deepest
 * path first (≤ 5), with their members in name order (self excluded). `everyone`: name order (≤ 100,
 * self excluded); `total` above `items.length` means only a search finds the rest. With
 * `directoryAvailable: false`, `teams` and `everyone` are empty and only `recent` is known.
 */
export type Recipients = {
  me: { personId: string; groups: RecipientGroup[] }
  recent: Candidate[]
  teams: { group: RecipientGroup; members: Candidate[] }[]
  everyone: { total: number; items: Candidate[] }
  directoryAvailable: boolean
}
export type Value = { key: string; label: string; description: string | null }
/** One week's frozen (or, before its first write, current) limits. */
export type Period = { key: string; startsAt: string; endsAt: string; allowance: number; maxPointsPerPost: number }
export type Program = {
  timeZone: string
  timeZoneLocked: boolean
  weeklyAllowance: number
  maxPointsPerPost: number
  clapCost: number
  values: Value[]
  updatedAt: string
}
/** Program plus the money settings only an admin reads. `pointUnitValue` is a decimal string. */
export type ProgramAdmin = Program & { pointUnitValue: string; currency: string }
/** `clappedByMe`: how many times the caller clapped this post. `clappers`: the newest three who clapped. */
export type Post = {
  id: string
  sender: Member
  recipient: Member
  points: number
  message: string
  values: string[]
  clapCount: number
  clapPoints: number
  clappedByMe: number
  clappers: Member[]
  period: string
  month: string
  createdAt: string
}
export type Clap = { id: string; member: Member; createdAt: string }
export type MonthStats = { key: string; receivedPoints: number; receivedPosts: number; sentPosts: number }
export type TotalStats = { receivedPoints: number; receivedPosts: number; sentPosts: number }
export type PeriodStats = { key: string; startsAt: string; endsAt: string; allowance: number; remaining: number; sentPosts: number; sentPoints: number; claps: number }
export type MyStats = { period: PeriodStats; month: MonthStats; total: TotalStats }
/** A counterpart by points (top five over all time on a member page, of the month in insights). */
export type Partner = { member: Member; posts: number; points: number }
/** Posts (and their points) carrying one value key. */
export type ValueCount = { key: string; posts: number; points: number }
/** One month of a member's activity (`key` = `YYYY-MM`). */
export type MonthPoint = { key: string; receivedPoints: number; receivedPosts: number; sentPosts: number }
/** One ISO week overlapping an insights month (`key` = `YYYY-Www`). */
export type WeekPoint = { key: string; startsAt: string; endsAt: string; posts: number; points: number; claps: number }
/**
 * `partners`: top five by points over all time. `values`: received posts per value key, posts descending.
 * `months`: the current month and the five before it, oldest first, zero-filled.
 */
export type MemberStats = {
  month: MonthStats
  total: TotalStats
  partners: { sentTo: Partner[]; receivedFrom: Partner[] }
  values: ValueCount[]
  months: MonthPoint[]
}
/**
 * The caller (upserted), their roles, the Program, this week's limits and their stats.
 * `period.maxPointsPerPost` is this week's cap (it may differ from the Program after a midweek change).
 * `directory.linked`: Directory answered; `publicUrl` is its browser origin, null when unset.
 */
export type Me = {
  member: Member
  roles: { peerbonusAdmin: boolean }
  program: Program
  period: Period
  stats: MyStats
  /** `unread`: inbox items newer than the caller's watermark. */
  inbox: { unread: number }
  directory: { linked: boolean; publicUrl: string | null }
}
/** `groups`: the person's direct Directory groups (empty while Directory is unavailable). */
export type MemberProfile = { member: Member; groups: RecipientGroup[]; stats: MemberStats }
/**
 * `kind: 'post'`: `post` was sent to the caller, `actor` is its sender, `id` is the post id.
 * `kind: 'clap'`: `actor` clapped `post` (sent or received by the caller), `id` is the clap id.
 */
export type InboxItem = { id: string; kind: 'post' | 'clap'; post: Post; actor: Member; createdAt: string; unread: boolean }
/** One inbox page, newest first; `seenAt` is the caller's watermark (null before the first visit). */
export type InboxPage = { items: InboxItem[]; nextCursor: string | null; seenAt: string | null }
/** One timeline page; `nextCursor` is opaque and null on the last page. */
export type PostPage = { items: Post[]; nextCursor: string | null }
export type StatementLine = { member: Member; points: number; amount: string }
export type Statement = {
  month: string
  closed: boolean
  closedAt: string | null
  closedBy: Member | null
  pointUnitValue: string
  currency: string
  totalPoints: number
  totalAmount: string
  memberCount: number
  directoryRefreshed: boolean
  lines: StatementLine[]
  /** Why the month cannot be closed now; empty when it can (or already is). */
  closeBlockers: StatementCloseBlocker[]
  /** The earliest still-open month before this one, when that blocks the close. */
  previousOpenMonth: string | null
}
export type StatementCloseBlocker = 'month_not_ended' | 'previous_month_open'
export type StatementSummary = { month: string; closed: boolean; closedAt: string | null; totalPoints: number; totalAmount: string; memberCount: number }
/** One Directory person's activity in a month; `member` is null until they gave or received points. `avatarUrl`: as on `Member`. */
export type Participant = {
  personId: string
  displayName: string
  email: string
  pending: boolean
  avatarUrl: string | null
  member: Member | null
  sentPosts: number
  sentPoints: number
  receivedPosts: number
  receivedPoints: number
  clapsGiven: number
}
/** Everyone in Directory (or, while it is down, every member with activity) for one month. */
export type Participation = {
  month: string
  directoryAvailable: boolean
  summary: { people: number; senders: number; recipients: number; active: number }
  items: Participant[]
}
/** One month's totals, per-value and per-week series, and the top five recipients and senders by points. */
export type Insights = {
  month: string
  posts: number
  points: number
  claps: number
  senders: number
  recipients: number
  values: ValueCount[]
  weeks: WeekPoint[]
  topRecipients: Partner[]
  topSenders: Partner[]
}
/** A fact about one node a manager acts on (design "v3: network map"); unknown strings are ignored. */
export type NetworkSignal = 'isolated' | 'fewConnections' | 'manyConnections' | 'concentrated' | 'declining' | 'unclapped'
/** Release constants echoed by the server so the UI never hardcodes a number. */
export type NetworkThresholds = { manyConnections: number; fewConnections: number; concentrationPosts: number; concentrationShare: number; declineRatio: number; declineMinimum: number }
/** A Directory group for the network picker. */
export type NetworkGroup = { id: string; name: string; path: string }
/**
 * One Directory person (or a member Directory no longer lists). `memberId` is null until they ever sent or
 * received. `connections`: distinct counterparts in the month, Project-wide. `share`: receivedPosts / the
 * month's posts. `decline`: own actions (sent posts + claps) in the 14 days ending at `asOf` vs the 14 before,
 * null when the previous window had fewer than `declineMinimum`.
 */
export type NetworkNode = {
  personId: string
  memberId: string | null
  displayName: string
  email: string
  pending: boolean
  avatarUrl: string | null
  connections: number
  sentPosts: number
  receivedPosts: number
  receivedPoints: number
  clapsGiven: number
  clapsReceived: number
  unclappedPosts: number
  share: number
  decline: { recent: number; previous: number } | null
  signals: NetworkSignal[]
}
/** An undirected pair (`from` < `to` by personId) that exchanged posts or claps in the month; `lastAt`: the newest. */
export type NetworkEdge = { from: string; to: string; posts: number; claps: number; points: number; lastAt: string }
/**
 * `GET /admin/network`: who is connected to whom in one month. `recentSince`: start of the current ISO week
 * when the month is current (edges with `lastAt >= recentSince` are "this week"), else null. `group`: the
 * applied filter; `groups`: every Directory group. With `directoryAvailable: false` only members with activity
 * are nodes, `groups` is empty and the filter is ignored. Every edge end is a node.
 */
export type Network = {
  month: string
  asOf: string
  recentSince: string | null
  directoryAvailable: boolean
  group: NetworkGroup | null
  groups: NetworkGroup[]
  thresholds: NetworkThresholds
  summary: { people: number; connected: number; isolated: number; posts: number; claps: number }
  nodes: NetworkNode[]
  edges: NetworkEdge[]
}
/** `GET /config`: the server's Project and browser origins (`publicUrl` / `directoryUrl` empty when unset). */
export type AppConfig = { projectId: string; publicUrl: string; directoryUrl: string }
/**
 * The Slack connection (design slack-integration §3.3): `unconfigured` = no bot token in the server's
 * environment; `configured` = token present, workspace not verified; `connected` = workspace fixed and the
 * token agrees; `team_changed` = the token now belongs to another workspace, deliveries are paused.
 */
export type SlackState = 'unconfigured' | 'configured' | 'connected' | 'team_changed'
export type SlackLocale = 'ja' | 'en'
export type SlackChannelRef = { id: string; name: string }
/** A public channel; `isMember`: the bot is in it (only then can it post there). */
export type SlackChannel = SlackChannelRef & { isMember: boolean }
export type SlackSettings = {
  announceChannel: SlackChannelRef | null
  announcePosts: boolean
  dmRecipientOnPost: boolean
  giveFromSlack: boolean
  locale: SlackLocale
}
/** The Slack inbound endpoint the platform projected (Phase B); `url` is null until it is ready. */
export type SlackInbound = { url: string | null; ready: boolean; reason: string | null }
/** `credential`: which Dashboard credential slots hold a value (never the values). `inbound`: null before Phase B. */
export type SlackAdmin = {
  credential: { botToken: boolean; signingSecret: boolean }
  state: SlackState
  workspace: { teamId: string; teamName: string; botUserId: string; verifiedAt: string } | null
  settings: SlackSettings
  inbound: SlackInbound | null
  lastError: { code: string; at: string } | null
  deliveries: { pending: number; failed24h: number; sent24h: number }
}
export type SlackDeliveryKind = 'post_announce' | 'post_dm' | 'clap_update' | 'post_delete'
export type SlackDeliveryState = 'pending' | 'sent' | 'skipped' | 'failed'
/** One outbox row, newest first. `lastError`: the Slack error or skip reason. */
export type SlackDelivery = {
  id: number
  kind: SlackDeliveryKind
  postId: string
  state: SlackDeliveryState
  attempts: number
  lastError: string | null
  createdAt: string
  sentAt: string | null
}
/** `announceChannelId: null` stops announcing to any channel. */
export type SlackSettingsPatch = Partial<Omit<SlackSettings, 'announceChannel'>> & { announceChannelId?: string | null }
/** The two workspaces of a refused verify (409 `slack_team_changed`). */
export type SlackTeamChange = { current: { teamId: string; teamName: string }; token: { teamId: string; teamName: string } }
export type ApiError = { error: string; message: string }

export type CreatePostInput = { recipientPersonId: string; points: number; message: string; values?: string[] }
export type ClapResult = { clap: Clap; post: Post }
/** `values` replaces the whole list. */
export type ProgramPatch = Partial<Pick<ProgramAdmin, 'timeZone' | 'weeklyAllowance' | 'maxPointsPerPost' | 'pointUnitValue' | 'currency' | 'values'>>
/**
 * `limit` 1–50 (server default 20). `member`: sent or received by; `sent` / `received`: one direction;
 * `value`: carries the value key; `period` / `month`: created in that week / month; `q`: text search.
 */
export type TimelineParams = {
  limit?: number
  cursor?: string | null
  member?: string
  sent?: string
  received?: string
  value?: string
  period?: string
  month?: string
  /** Case-insensitive substring of the message, sender or recipient name. */
  q?: string
}

/**
 * Non-2xx peerbonus response. `code` is the server's stable snake_case error;
 * `body` keeps extra fields (`remaining` / `required` of `insufficient_points`).
 * `transient` = no error envelope came back: a 5xx from a proxy in front of the
 * server (e.g. during a rollout), or no response at all (status 0).
 */
export class PeerbonusApiError extends Error {
  readonly status: number
  readonly code: string
  readonly transient: boolean
  readonly body: Readonly<Record<string, unknown>>

  constructor(status: number, body: (Partial<ApiError> & Record<string, unknown>) | undefined) {
    const code = typeof body?.error === 'string' ? body.error : undefined
    super((typeof body?.message === 'string' && body.message) || `peerbonus api ${status}`)
    this.name = 'PeerbonusApiError'
    this.status = status
    this.code = code ?? (status === 404 ? 'not_found' : 'unknown')
    this.transient = code === undefined && (status === 0 || status >= 500)
    this.body = body ?? {}
  }
}

export function isPeerbonusError(error: unknown, code?: string): error is PeerbonusApiError {
  return error instanceof PeerbonusApiError && (code === undefined || error.code === code)
}

/** The balance and the cost of a refused send or clap (409 `insufficient_points`), else undefined. */
export function insufficientPoints(error: unknown): { remaining: number; required: number } | undefined {
  if (!isPeerbonusError(error, 'insufficient_points')) return undefined
  const { remaining, required } = error.body
  return typeof remaining === 'number' && typeof required === 'number' ? { remaining, required } : undefined
}

/** A fresh `Idempotency-Key` (UUID v4) for one send or clap attempt; a retry of the same attempt reuses it. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID()
}

export const BASE = '/api/peerbonus/v1'
const READ_TIMEOUT_MS = 8_000
// A 401 first sends the browser to `/` so the edge re-runs sign-in. Another 401
// within this window means the edge forwards a token the server rejects;
// redirecting again would loop, so that 401 reaches the screens instead.
const REAUTH_WINDOW_MS = 60_000
const REAUTH_AT_KEY = 'peerbonus:reauth-at'

type Fetcher = (input: string, init: RequestInit) => Promise<Response>

export interface PeerbonusApi {
  /** The caller (upserted from Directory), roles, Program, this week and stats. 503 `directory_unavailable` before the first link. */
  me(): Promise<Me>
  program(): Promise<Program>
  /** The recipient picker's people: recent, the caller's teams, everyone (Directory down: recent only). */
  recipients(): Promise<Recipients>
  members: {
    /**
     * Directory people matching `q` (up to 64 characters; empty: everyone in name order), the caller
     * excluded; `limit` 1–20 (default 10). 503 `directory_unavailable` while Directory is down.
     */
    search(q: string, limit?: number): Promise<Candidate[]>
    get(memberId: string): Promise<MemberProfile>
  }
  inbox: {
    /** Received posts and claps on the caller's posts (never their own claps), newest first. */
    list(params?: { limit?: number; cursor?: string | null }): Promise<InboxPage>
    /** Moves the caller's watermark to now; every current item becomes read. */
    markSeen(): Promise<{ seenAt: string; unread: number }>
  }
  posts: {
    /** Newest first. 400 `invalid_request` for a bad cursor, limit or key. */
    list(params?: TimelineParams): Promise<PostPage>
    /** A deleted post is 404. */
    get(postId: string): Promise<Post>
    /**
     * 201 with the post; the same key again answers the same post. 400 `invalid_request` / `self_target` /
     * `unknown_value`, 404 `recipient_not_found`, 409 `insufficient_points`, 503 `directory_unavailable`.
     */
    create(input: CreatePostInput, idempotencyKey: string): Promise<Post>
    /** Admin only (403 `forbidden`); 204. */
    remove(postId: string): Promise<void>
    /** Newest first, at most 50. */
    claps(postId: string): Promise<Clap[]>
    /** 2 points from the caller. 403 `own_post`, 404, 409 `insufficient_points` / `period_closed`. */
    clap(postId: string, idempotencyKey: string): Promise<ClapResult>
  }
  admin: {
    program(): Promise<ProgramAdmin>
    /** 409 `timezone_locked` once any points moved. */
    updateProgram(patch: ProgramPatch): Promise<ProgramAdmin>
    /** Every month from the first ledger entry to now, newest first. */
    statements(): Promise<StatementSummary[]>
    /** An open month is the current preview (`closed: false`). */
    statement(month: string): Promise<Statement>
    /** Idempotent. 409 `month_not_ended` / `previous_month_open`, 400 `invalid_month`. */
    closeStatement(month: string): Promise<Statement>
    /** Participation in `month` (server default: the current month), most received first. */
    members(month?: string): Promise<Participation>
    /** One month's totals and series (`YYYY-MM`, required). */
    insights(month: string): Promise<Insights>
    /** The month's connection graph and signals; `group` narrows the nodes. 404 `group_not_found`. */
    network(month: string, group?: string): Promise<Network>
    slack: {
      get(): Promise<SlackAdmin>
      /**
       * 400 `invalid_request` (unknown channel, `announcePosts` without a channel), 409 `slack_not_connected`,
       * 409 `slack_signing_secret_missing` (`giveFromSlack: true` without the signing secret).
       */
      update(patch: SlackSettingsPatch): Promise<SlackAdmin>
      /**
       * `auth.test`, then fixes (or refreshes) the workspace. 409 `slack_not_configured`, 409 `slack_team_changed`
       * (`detail`: SlackTeamChange) unless `force`, which re-fixes and forgets every member's Slack user. 502 `slack_error`.
       */
      verify(force?: boolean): Promise<SlackAdmin>
      /** Public channels whose name starts with `q` (≤ 20). */
      channels(q: string): Promise<SlackChannel[]>
      /** A test post to the saved announce channel. 409 `slack_bot_not_in_channel` / `slack_not_connected`, 502 `slack_error`. */
      test(): Promise<{ ok: true; ts: string }>
      /** Newest first, ≤ 50; `state` narrows to one state. */
      deliveries(state?: SlackDeliveryState): Promise<SlackDelivery[]>
    }
  }
  /** `GET /config` (outside `/api/peerbonus/v1`). */
  config(): Promise<AppConfig>
}

/** `detail` of a 409 `slack_team_changed`, else undefined. */
export function slackTeamChange(error: unknown): SlackTeamChange | undefined {
  if (!isPeerbonusError(error, 'slack_team_changed')) return undefined
  const detail = error.body.detail as Partial<SlackTeamChange> | undefined
  return detail?.current?.teamId && detail.token?.teamId ? (detail as SlackTeamChange) : undefined
}

/** `detail` of a 502 `slack_error` (Slack's own error code, e.g. `invalid_auth`), else undefined. */
export function slackErrorDetail(error: unknown): string | undefined {
  if (!isPeerbonusError(error, 'slack_error')) return undefined
  return typeof error.body.detail === 'string' ? error.body.detail : undefined
}

/** `?limit=&cursor=&member=…` with only the set parameters, in a stable order. */
export function timelineSearch(params: TimelineParams = {}): string {
  const search = new URLSearchParams()
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.cursor) search.set('cursor', params.cursor)
  for (const name of ['member', 'sent', 'received', 'value', 'period', 'month', 'q'] as const) {
    const value = params[name]
    if (value) search.set(name, value)
  }
  const text = search.toString()
  return text ? `?${text}` : ''
}

/** The closed month's payroll CSV (a plain download link; the browser sends the session cookie). */
export function statementCsvUrl(month: string): string {
  return `${BASE}/admin/statements/${encodeURIComponent(month)}/export.csv`
}

export function createPeerbonusApi(fetcher: Fetcher = (input, init) => fetch(input, init), reauth = browserReauth): PeerbonusApi {
  /** `path` is under BASE unless `root` (`/config`). */
  async function call<T>(method: string, path: string, options: { body?: unknown; idempotencyKey?: string; root?: boolean } = {}): Promise<T> {
    const read = method === 'GET'
    const headers: Record<string, string> = { Accept: 'application/json' }
    // The server requires `Content-Type: application/json` on every change, body or not (api.md §1).
    if (!read) headers['Content-Type'] = 'application/json'
    if (options.idempotencyKey) headers['Idempotency-Key'] = options.idempotencyKey
    let response: Response
    try {
      response = await fetcher(options.root ? path : `${BASE}${path}`, {
        method,
        headers,
        body: options.body === undefined ? undefined : JSON.stringify(options.body),
        credentials: 'same-origin',
        signal: read ? AbortSignal.timeout(READ_TIMEOUT_MS) : undefined,
      })
    } catch {
      // Network failure or read timeout: nothing answered.
      throw new PeerbonusApiError(0, undefined)
    }
    if (response.status === 401 && reauth()) return new Promise<T>(() => {})
    if (!response.ok) {
      let parsed: (Partial<ApiError> & Record<string, unknown>) | undefined
      try {
        const json: unknown = await response.json()
        parsed = json && typeof json === 'object' && !Array.isArray(json) ? (json as Record<string, unknown>) : undefined
      } catch {
        parsed = undefined
      }
      throw new PeerbonusApiError(response.status, parsed)
    }
    if (response.status === 204) return undefined as T
    return (await response.json()) as T
  }
  const list = async <T>(path: string) => (await call<{ items: T[] }>('GET', path)).items
  const id = encodeURIComponent

  return {
    me: () => call<Me>('GET', '/me'),
    program: () => call<Program>('GET', '/program'),
    recipients: () => call<Recipients>('GET', '/recipients'),
    members: {
      search: (q, limit) => {
        const search = new URLSearchParams()
        if (q) search.set('q', q)
        if (limit !== undefined) search.set('limit', String(limit))
        const query = search.toString()
        return list<Candidate>(`/members${query ? `?${query}` : ''}`)
      },
      get: (memberId) => call<MemberProfile>('GET', `/members/${id(memberId)}`),
    },
    inbox: {
      list: (params = {}) => {
        const search = new URLSearchParams()
        if (params.limit !== undefined) search.set('limit', String(params.limit))
        if (params.cursor) search.set('cursor', params.cursor)
        const query = search.toString()
        return call<InboxPage>('GET', `/inbox${query ? `?${query}` : ''}`)
      },
      markSeen: () => call<{ seenAt: string; unread: number }>('POST', '/inbox/seen'),
    },
    posts: {
      list: (params) => call<PostPage>('GET', `/posts${timelineSearch(params)}`),
      get: (postId) => call<Post>('GET', `/posts/${id(postId)}`),
      create: (input, idempotencyKey) => call<Post>('POST', '/posts', { body: input, idempotencyKey }),
      remove: (postId) => call<void>('DELETE', `/posts/${id(postId)}`),
      claps: (postId) => list<Clap>(`/posts/${id(postId)}/claps`),
      clap: (postId, idempotencyKey) => call<ClapResult>('POST', `/posts/${id(postId)}/claps`, { idempotencyKey }),
    },
    admin: {
      program: () => call<ProgramAdmin>('GET', '/admin/program'),
      updateProgram: (patch) => call<ProgramAdmin>('PATCH', '/admin/program', { body: patch }),
      statements: () => list<StatementSummary>('/admin/statements'),
      statement: (month) => call<Statement>('GET', `/admin/statements/${id(month)}`),
      closeStatement: (month) => call<Statement>('POST', `/admin/statements/${id(month)}/close`),
      members: (month) => call<Participation>('GET', `/admin/members${month ? `?month=${id(month)}` : ''}`),
      insights: (month) => call<Insights>('GET', `/admin/insights?month=${id(month)}`),
      network: (month, group) => call<Network>('GET', `/admin/network?month=${id(month)}${group ? `&group=${id(group)}` : ''}`),
      slack: {
        get: () => call<SlackAdmin>('GET', '/admin/slack'),
        update: (patch) => call<SlackAdmin>('PATCH', '/admin/slack', { body: patch }),
        verify: (force = false) => call<SlackAdmin>('POST', '/admin/slack/verify', { body: force ? { force: true } : {} }),
        channels: (q) => list<SlackChannel>(`/admin/slack/channels${q ? `?q=${id(q)}` : ''}`),
        test: () => call<{ ok: true; ts: string }>('POST', '/admin/slack/test'),
        deliveries: (state) => list<SlackDelivery>(`/admin/slack/deliveries${state ? `?state=${id(state)}` : ''}`),
      },
    },
    config: () => call<AppConfig>('GET', '/config', { root: true }),
  }
}

function browserReauth(): boolean {
  const lastReauthAt = Number(sessionStorage.getItem(REAUTH_AT_KEY))
  if (Date.now() - lastReauthAt < REAUTH_WINDOW_MS) return false
  sessionStorage.setItem(REAUTH_AT_KEY, String(Date.now()))
  window.location.assign('/')
  return true
}

export const peerbonusApi = createPeerbonusApi()
