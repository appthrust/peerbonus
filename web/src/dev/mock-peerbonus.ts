// Dev-only in-memory implementation of the peerbonus participant contract
// (`/api/peerbonus/v1/*`, cmd/server; api.md) plus the server's
// `GET /config`. Admin routes are mounted by ./mock-admin through
// `registerMockRoutes`.
//
// Fixtures: nine Directory people in four groups. Eight are Members (鈴木 never
// signed in, so `pending`); 小林 exists only in Directory until someone sends
// them points. 山田 belongs directly to `sales/team-a` (佐藤, 鈴木, 田中) and
// `projects/ceatec` (伊藤, 中村), small enough for the picker to stack them;
// 加藤 to the parent `sales`, whose effective members include both teams; Ken
// Watanabe to no group (the empty-team view).
// About fifty posts spread over the current and the two previous months (more
// than one timeline page), claps from uninvolved members, and the append-only
// ledger both produce. Seeds are relative to the mock clock at reset time.
// The caller is 山田 (peerbonus admin) unless MOCK_ROLE is set to anything but
// Admin. `POST /__mock/caller {memberId, admin?}` switches the caller (default
// not admin); `POST /__mock/directory {up}` takes Directory down to exercise
// 503 `directory_unavailable` and `GET /recipients` with `directoryAvailable:
// false`; `POST /__mock/reset` reseeds everything.
// The timeline cursor is base64url(`createdAt|id`), opaque to the SPA.
// v2: the inbox derives events from visible posts/claps and owner watermarks;
// profiles derive partners, received values and six calendar months on read.
import type { ApiError, Candidate, Clap, InboxItem, Member, MemberStats, Me, MyStats, Partner, Period, Post, Program, RecipientGroup, Recipients, Value, ValueCount } from '../api/peerbonus'
import { monthKeyAt, periodAt, previousMonth } from '../model/period'
import { CLAP_COST, MAX_VALUES, messageProblem, VALUE_KEY } from '../model/post'
import { mockAvatarPng, type Rgb } from './mock-images'

/** `groups`: the ids of the Directory groups the person belongs to directly. */
export type MockPerson = { personId: string; subject: string | null; displayName: string; email: string; groups: string[]; avatarVersion: string | null; avatarColor: Rgb | null }
export type MockMember = { id: string; personId: string; subject: string | null; displayName: string; email: string; avatarVersion: string | null }
export type MockProgram = {
  timeZone: string
  weeklyAllowance: number
  maxPointsPerPost: number
  pointUnitValue: string
  currency: string
  values: Value[]
  updatedAt: string
  updatedBy: string | null
}
export type MockPost = {
  id: string
  periodKey: string
  monthKey: string
  senderId: string
  recipientId: string
  points: number
  message: string
  values: string[]
  createdAt: string
  deletedAt: string | null
  deletedBy: string | null
  reversed: boolean
}
export type MockClap = { id: string; postId: string; memberId: string; periodKey: string; monthKey: string; createdAt: string }
export type LedgerKind = 'send' | 'receive' | 'clap_give' | 'clap_sender' | 'clap_recipient' | 'reversal'
export type MockLedger = {
  id: number
  memberId: string
  periodKey: string
  monthKey: string
  account: 'give' | 'receive'
  delta: number
  kind: LedgerKind
  postId: string | null
  clapId: string | null
  reversesId: number | null
  createdAt: string
}
/** A closed month as ./mock-admin freezes it; its presence alone closes the month here. */
export type MockStatement = {
  month: string
  closedAt: string
  closedBy: string
  pointUnitValue: string
  currency: string
  directoryRefreshed: boolean
  lines: { memberId: string; points: number; amount: string; pending: boolean }[]
}
export type MockRouteHandler = (request: Request, url: URL) => Promise<Response | undefined> | Response | undefined
/** Everything the mock knows; `mockState()` returns the live object (replaced on reset). */
export type MockState = {
  /** Directory's people (the search and recipient source). */
  people: MockPerson[]
  /** Directory's groups; a group's effective members include its subgroups' (`path` prefix). */
  groups: RecipientGroup[]
  members: MockMember[]
  program: MockProgram
  /** Frozen weeks by key, written at a week's first send or clap. */
  periods: Record<string, Period>
  posts: MockPost[]
  claps: MockClap[]
  ledger: MockLedger[]
  /** Closed months by key. */
  statements: Record<string, MockStatement>
  /** `${memberId}|${key}` → the post or clap the key created. */
  idempotency: Record<string, { postId: string; clapId: string | null }>
  /** Owner-specific inbox watermarks; absent means never seen. */
  inboxSeenAt: Record<string, string>
  callerId: string
  admin: boolean
  directoryUp: boolean
  seq: number
  lastStamp: number
}

const BASE = '/api/peerbonus/v1'
const DEFAULT_LIMIT = 20
const MAX_LIMIT = 50
const MAX_CLAPS = 50
const DIRECTORY_PUBLIC_URL = process.env.PEERBONUS_MOCK_DIRECTORY_PUBLIC_URL ?? 'https://directory.pj-mock.appthrust.ink'

/** `GET /config` as cmd/server answers it. */
export const mockConfig = {
  projectId: 'pj-mock',
  publicUrl: 'https://peerbonus.pj-mock.appthrust.ink',
  directoryUrl: DIRECTORY_PUBLIC_URL,
}

const CROCKFORD = '0123456789ABCDEFGHJKMNPQRSTVWXYZ'

/** A ULID-shaped id: 48-bit time and an 80-bit tail derived from `seq`, both Crockford base32. */
function ulid(time: number, seq: number): string {
  let head = ''
  for (let value = time, i = 0; i < 10; i++, value = Math.floor(value / 32)) head = CROCKFORD[value % 32] + head
  let tail = ''
  for (let value = seq * 2654435761, i = 0; i < 16; i++, value = Math.floor(value / 32) + i * 7919) tail = CROCKFORD[value % 32] + tail
  return head + tail
}

/** Fixed member ids, so tests and links can name them. */
export const MOCK_MEMBER_IDS = {
  yamada: '01J9PBMEMBER0000000YAMADA0',
  sato: '01J9PBMEMBER00000000SAT0O0',
  suzuki: '01J9PBMEMBER0000000SUZUK10',
  tanaka: '01J9PBMEMBER0000000TANAKA0',
  kato: '01J9PBMEMBER000000000KAT00',
  ito: '01J9PBMEMBER0000000000ITO0',
  watanabe: '01J9PBMEMBER000000WATANABE',
  nakamura: '01J9PBMEMBER000000NAKAMURA',
} as const
type MemberName = keyof typeof MOCK_MEMBER_IDS

/** Directory person ids; `kobayashi` has no Member until someone sends them points. */
export const MOCK_PERSON_IDS = {
  yamada: 'p-yamada',
  sato: 'p-sato',
  suzuki: 'p-suzuki',
  tanaka: 'p-tanaka',
  kato: 'p-kato',
  ito: 'p-ito',
  watanabe: 'p-watanabe',
  nakamura: 'p-nakamura',
  kobayashi: 'p-kobayashi',
} as const

/** Directory groups (`/people/{id}` memberships, `/people?groupId=`). */
export const MOCK_GROUPS = {
  sales: { id: 'g-sales', name: '営業部', path: 'sales' },
  teamA: { id: 'g-sales-team-a', name: '営業1課', path: 'sales/team-a' },
  teamB: { id: 'g-sales-team-b', name: '営業2課', path: 'sales/team-b' },
  ceatec: { id: 'g-projects-ceatec', name: 'CEATEC 出展', path: 'projects/ceatec' },
} as const satisfies Record<string, RecipientGroup>
const G = MOCK_GROUPS

const PEOPLE: readonly MockPerson[] = [
  { personId: MOCK_PERSON_IDS.yamada, subject: 'sub-yamada', displayName: '山田 太郎', email: 'yamada@example.com', groups: [G.teamA.id, G.ceatec.id], avatarVersion: '2026-10-01T09:00:00.000Z', avatarColor: [37, 99, 235] },
  { personId: MOCK_PERSON_IDS.sato, subject: 'sub-sato', displayName: '佐藤 花子', email: 'sato@example.com', groups: [G.teamA.id], avatarVersion: '2026-10-01T09:00:00.000Z', avatarColor: [219, 39, 119] },
  { personId: MOCK_PERSON_IDS.suzuki, subject: null, displayName: '鈴木 一郎', email: 'suzuki@example.com', groups: [G.teamA.id], avatarVersion: null, avatarColor: null },
  { personId: MOCK_PERSON_IDS.tanaka, subject: 'sub-tanaka', displayName: '田中 美咲', email: 'tanaka@example.com', groups: [G.teamA.id], avatarVersion: '2026-10-01T09:00:00.000Z', avatarColor: [124, 58, 237] },
  { personId: MOCK_PERSON_IDS.kato, subject: 'sub-kato', displayName: '加藤 大輔', email: 'kato@example.com', groups: [G.sales.id], avatarVersion: '2026-10-01T09:00:00.000Z', avatarColor: [5, 150, 105] },
  { personId: MOCK_PERSON_IDS.ito, subject: 'sub-ito', displayName: '伊藤 さくら', email: 'ito@example.com', groups: [G.teamB.id, G.ceatec.id], avatarVersion: '2026-10-01T09:00:00.000Z', avatarColor: [234, 88, 12] },
  { personId: MOCK_PERSON_IDS.watanabe, subject: 'sub-watanabe', displayName: 'Ken Watanabe', email: 'watanabe@example.com', groups: [], avatarVersion: null, avatarColor: null },
  { personId: MOCK_PERSON_IDS.nakamura, subject: 'sub-nakamura', displayName: '中村 翔', email: 'nakamura@example.com', groups: [G.ceatec.id], avatarVersion: '2026-10-01T09:00:00.000Z', avatarColor: [8, 145, 178] },
  { personId: MOCK_PERSON_IDS.kobayashi, subject: null, displayName: '小林 優', email: 'kobayashi@example.com', groups: [G.teamB.id], avatarVersion: '2026-10-01T09:00:00.000Z', avatarColor: [202, 138, 4] },
]

const VALUES: readonly Value[] = [
  { key: 'thanks', label: '感謝', description: null },
  { key: 'challenge', label: '挑戦', description: '前例のないことに踏み出す' },
  { key: 'teamwork', label: '協力', description: '仲間を助け、助けを求める' },
  { key: 'learning', label: '学び', description: null },
  { key: 'customer', label: '顧客志向', description: null },
]

const MESSAGES = [
  'リリース前のレビュー、夜遅くまで付き合ってくれてありがとう！おかげで安心して出せました。',
  '障害対応の切り分けが本当に速かった。手順書も残してくれて助かります。',
  'お客様との打ち合わせで論点をきれいに整理してくれました。次回もぜひ一緒に。',
  '新人向けの勉強会資料、とても分かりやすかったです。 https://example.com/slides',
  '急な仕様変更にも嫌な顔ひとつせず対応してくれて感謝です🙏',
  'テストの自動化、地味だけど効いてます。CI が 5 分速くなりました！',
  'ペアプロで詰まっていたところを一緒に解いてくれてありがとう。',
  'デザインの細かいところまで見てくれて、画面がぐっと良くなりました。',
] as const

const POINTS = [10, 20, 30, 50, 15, 40, 5, 80] as const
const MINUTE = 60_000
const HOUR = 60 * MINUTE

let clockOverride: number | null = null

/** The mock's "now": the override when a test set one, else the wall clock. */
export function mockNow(): number {
  return clockOverride ?? Date.now()
}

/** Pins the mock clock (`null` returns to the wall clock). Reset reseeds relative to it. */
export function setMockClock(ms: number | null) {
  clockOverride = ms
}

function seed(): MockState {
  const start = mockNow()
  const program: MockProgram = {
    timeZone: 'Asia/Tokyo',
    weeklyAllowance: 400,
    maxPointsPerPost: 120,
    pointUnitValue: '1',
    currency: 'JPY',
    values: VALUES.map((value) => ({ ...value })),
    updatedAt: new Date(start - 90 * 24 * HOUR).toISOString(),
    updatedBy: null,
  }
  const names = Object.keys(MOCK_MEMBER_IDS) as MemberName[]
  const members: MockMember[] = names.map((name) => {
    const person = PEOPLE.find((candidate) => candidate.personId === MOCK_PERSON_IDS[name])!
    return { id: MOCK_MEMBER_IDS[name], personId: person.personId, subject: person.subject, displayName: person.displayName, email: person.email, avatarVersion: person.avatarVersion }
  })
  const fresh: MockState = {
    people: PEOPLE.map((person) => ({ ...person, groups: [...person.groups] })),
    groups: Object.values(MOCK_GROUPS).map((group) => ({ ...group })),
    members,
    program,
    periods: {},
    posts: [],
    claps: [],
    ledger: [],
    statements: {},
    idempotency: {},
    inboxSeenAt: { [MOCK_MEMBER_IDS.yamada]: new Date(start - 14 * 24 * HOUR).toISOString() },
    callerId: MOCK_MEMBER_IDS.yamada,
    admin: (process.env.MOCK_ROLE ?? 'Admin') === 'Admin',
    directoryUp: true,
    seq: 100,
    lastStamp: 0,
  }
  state = fresh
  // Oldest first, so the ledger ids grow with time. 48 posts every 35 hours reach back about 70 days.
  for (let index = 47; index >= 0; index--) {
    const at = start - index * 35 * HOUR - 2 * HOUR
    const sender = MOCK_MEMBER_IDS[names[(index * 3) % names.length]!]
    let recipient = MOCK_MEMBER_IDS[names[(index * 5 + 1) % names.length]!]
    if (recipient === sender) recipient = MOCK_MEMBER_IDS[names[(index * 5 + 2) % names.length]!]
    const values = Array.from({ length: index % 4 }, (_, position) => VALUES[(index + position) % VALUES.length]!.key)
    const post = writePost(sender, recipient, POINTS[index % POINTS.length]!, MESSAGES[index % MESSAGES.length]!, values, at)
    // Up to two uninvolved members clap an hour later (never after "now").
    let clappers = 0
    for (const [offset, name] of names.entries()) {
      const clapper = MOCK_MEMBER_IDS[name]
      if (clappers === 2 || clapper === sender || clapper === recipient || (index + offset) % 4 !== 0) continue
      writeClap(post, clapper, Math.min(at + HOUR + offset * MINUTE, start - MINUTE))
      clappers++
    }
  }
  state.lastStamp = start
  return fresh
}

let state: MockState
seed()

/** The live mock state; ./mock-admin reads and writes it. Replaced on reset. */
export function mockState(): MockState {
  return state
}

/** Registered handlers (./mock-admin), tried before the participant routes. */
const extraRoutes: MockRouteHandler[] = []

/** Mounts more routes under the mock; handlers return undefined for requests they do not own. */
export function registerMockRoutes(handler: MockRouteHandler) {
  if (!extraRoutes.includes(handler)) extraRoutes.push(handler)
}

/** Strictly increasing write time, so two writes in one millisecond still order. */
function stamp(): number {
  state.lastStamp = Math.max(mockNow(), state.lastStamp + 1)
  return state.lastStamp
}

export const isAdmin = () => state.admin

function storedMember(memberId: string): MockMember {
  const member = state.members.find((candidate) => candidate.id === memberId)
  if (!member) throw new Error(`mock: unknown member ${memberId}`)
  return member
}

export const avatarUrl = (person: Pick<MockPerson, 'personId' | 'avatarVersion'>): string | null =>
  person.avatarVersion ? `${BASE}/avatars/${encodeURIComponent(person.personId)}?v=${encodeURIComponent(person.avatarVersion)}` : null

export function memberJson(memberId: string): Member {
  const member = storedMember(memberId)
  return {
    id: member.id,
    personId: member.personId,
    displayName: member.displayName,
    email: member.email,
    pending: member.subject === null,
    avatarUrl: avatarUrl(member),
    directoryUrl: DIRECTORY_PUBLIC_URL ? `${DIRECTORY_PUBLIC_URL.replace(/\/+$/, '')}/people/${encodeURIComponent(member.personId)}` : null,
  }
}

const hasLedger = () => state.ledger.length > 0

export function programJson(): Program {
  const { pointUnitValue: _unit, currency: _currency, updatedBy: _by, ...program } = state.program
  return { ...program, values: program.values.map((value) => ({ ...value })), timeZoneLocked: hasLedger(), clapCost: CLAP_COST }
}

/** This week's frozen row, else the Program's current limits (never inserted on read, B8). */
function periodFor(ms: number): Period {
  const bounds = periodAt(ms, state.program.timeZone)
  return state.periods[bounds.key] ?? { ...bounds, allowance: state.program.weeklyAllowance, maxPointsPerPost: state.program.maxPointsPerPost }
}

/** Freezes the week of `ms` at its first write. */
function freezePeriod(ms: number): Period {
  const period = periodFor(ms)
  state.periods[period.key] ??= period
  return period
}

function remaining(memberId: string, period: Period): number {
  let left = period.allowance
  for (const entry of state.ledger) if (entry.memberId === memberId && entry.account === 'give' && entry.periodKey === period.key) left += entry.delta
  return left
}

/** Points `memberId` received in `month` (net of reversals). */
export function received(memberId: string, month?: string): number {
  let total = 0
  for (const entry of state.ledger) if (entry.memberId === memberId && entry.account === 'receive' && (month === undefined || entry.monthKey === month)) total += entry.delta
  return total
}

function append(entry: Omit<MockLedger, 'id'>): MockLedger {
  const row = { ...entry, id: state.ledger.length + 1 }
  state.ledger.push(row)
  return row
}

function writePost(senderId: string, recipientId: string, points: number, message: string, values: string[], at: number): MockPost {
  const period = freezePeriod(at)
  const createdAt = new Date(at).toISOString()
  const post: MockPost = {
    id: ulid(at, state.seq++),
    periodKey: period.key,
    monthKey: monthKeyAt(at, state.program.timeZone),
    senderId,
    recipientId,
    points,
    message,
    values,
    createdAt,
    deletedAt: null,
    deletedBy: null,
    reversed: false,
  }
  state.posts.push(post)
  const common = { periodKey: post.periodKey, monthKey: post.monthKey, postId: post.id, clapId: null, reversesId: null, createdAt }
  append({ ...common, memberId: senderId, account: 'give', delta: -points, kind: 'send' })
  append({ ...common, memberId: recipientId, account: 'receive', delta: points, kind: 'receive' })
  return post
}

function writeClap(post: MockPost, memberId: string, at: number): MockClap {
  const period = freezePeriod(at)
  const clap: MockClap = { id: ulid(at, state.seq++), postId: post.id, memberId, periodKey: period.key, monthKey: monthKeyAt(at, state.program.timeZone), createdAt: new Date(at).toISOString() }
  state.claps.push(clap)
  const common = { periodKey: clap.periodKey, monthKey: clap.monthKey, postId: post.id, clapId: clap.id, reversesId: null, createdAt: clap.createdAt }
  append({ ...common, memberId, account: 'give', delta: -CLAP_COST, kind: 'clap_give' })
  append({ ...common, memberId: post.senderId, account: 'receive', delta: 1, kind: 'clap_sender' })
  append({ ...common, memberId: post.recipientId, account: 'receive', delta: 1, kind: 'clap_recipient' })
  return clap
}

function postJson(post: MockPost): Post {
  const claps = state.claps.filter((clap) => clap.postId === post.id).sort(timelineOrder)
  return {
    id: post.id,
    sender: memberJson(post.senderId),
    recipient: memberJson(post.recipientId),
    points: post.points,
    message: post.message,
    values: [...post.values],
    clapCount: claps.length,
    clapPoints: claps.length * CLAP_COST,
    clappedByMe: claps.filter((clap) => clap.memberId === state.callerId).length,
    clappers: claps.slice(0, 3).map((clap) => memberJson(clap.memberId)),
    period: post.periodKey,
    month: post.monthKey,
    createdAt: post.createdAt,
  }
}

const clapJson = (clap: MockClap): Clap => ({ id: clap.id, member: memberJson(clap.memberId), createdAt: clap.createdAt })

const live = () => state.posts.filter((post) => post.deletedAt === null)

/** Shared read-model aggregations; counts use visible posts, points use the ledger. */
export function valueCounts(posts: MockPost[]): ValueCount[] {
  const counts = new Map<string, ValueCount>()
  for (const post of posts) for (const key of post.values) {
    const row = counts.get(key) ?? { key, posts: 0, points: 0 }
    row.posts++
    row.points += post.points
    counts.set(key, row)
  }
  return [...counts.values()].sort((a, b) => b.posts - a.posts || a.key.localeCompare(b.key))
}

export function topPartners(posts: MockPost[], side: 'senderId' | 'recipientId'): Partner[] {
  const counts = new Map<string, Partner>()
  for (const post of posts) {
    const id = post[side]
    const row = counts.get(id) ?? { member: memberJson(id), posts: 0, points: 0 }
    row.posts++
    row.points += post.points
    counts.set(id, row)
  }
  return [...counts.values()].sort((a, b) => b.points - a.points || a.member.id.localeCompare(b.member.id)).slice(0, 5)
}
function memberStats(memberId: string, ms: number): MemberStats {
  const month = monthKeyAt(ms, state.program.timeZone)
  const posts = live()
  const receivedPosts = posts.filter((post) => post.recipientId === memberId)
  const sentPosts = posts.filter((post) => post.senderId === memberId)
  const monthKeys = [month]
  for (let i = 1; i < 6; i++) monthKeys.unshift(previousMonth(monthKeys[0]!))
  return {
    month: {
      key: month,
      receivedPoints: received(memberId, month),
      receivedPosts: posts.filter((post) => post.recipientId === memberId && post.monthKey === month).length,
      sentPosts: posts.filter((post) => post.senderId === memberId && post.monthKey === month).length,
    },
    total: {
      receivedPoints: received(memberId),
      receivedPosts: posts.filter((post) => post.recipientId === memberId).length,
      sentPosts: posts.filter((post) => post.senderId === memberId).length,
    },
    partners: { sentTo: topPartners(sentPosts, 'recipientId'), receivedFrom: topPartners(receivedPosts, 'senderId') },
    values: valueCounts(receivedPosts),
    months: monthKeys.map((key) => ({
      key,
      receivedPoints: received(memberId, key),
      receivedPosts: receivedPosts.filter((post) => post.monthKey === key).length,
      sentPosts: sentPosts.filter((post) => post.monthKey === key).length,
    })),
  }
}

function myStats(memberId: string, ms: number): MyStats {
  const period = periodFor(ms)
  const sent = live().filter((post) => post.senderId === memberId && post.periodKey === period.key)
  const { month, total } = memberStats(memberId, ms)
  return {
    period: {
      key: period.key,
      startsAt: period.startsAt,
      endsAt: period.endsAt,
      allowance: period.allowance,
      remaining: remaining(memberId, period),
      sentPosts: sent.length,
      sentPoints: sent.reduce((sum, post) => sum + post.points, 0),
      claps: state.claps.filter((clap) => clap.memberId === memberId && clap.periodKey === period.key).length,
    },
    month,
    total,
  }
}

/** Newest first; equal times order by id, both descending (the server's index order). */
const timelineOrder = (a: { createdAt: string; id: string }, b: { createdAt: string; id: string }) => b.createdAt.localeCompare(a.createdAt) || b.id.localeCompare(a.id)

function decodeCursor(cursor: string): { createdAt: string; id: string } | undefined {
  if (!/^[A-Za-z0-9_-]+$/.test(cursor)) return undefined
  const [createdAt, id, ...rest] = Buffer.from(cursor, 'base64url').toString('utf8').split('|')
  if (rest.length > 0 || !createdAt || !id || Number.isNaN(Date.parse(createdAt))) return undefined
  return { createdAt: new Date(createdAt).toISOString(), id }
}

export const json = (body: unknown, status = 200) => Response.json(body, { status, headers: { 'Cache-Control': 'no-store' } })
export const fail = (status: number, error: string, message: string, extra: Record<string, unknown> = {}) =>
  json({ error, message, ...extra } satisfies ApiError, status)
export const notFound = () => fail(404, 'not_found', 'The requested resource was not found')
export const forbidden = () => fail(403, 'forbidden', 'You do not have permission for this action')
export const invalid = (message: string) => fail(400, 'invalid_request', message)
const directoryUnavailable = () => fail(503, 'directory_unavailable', 'Directory is temporarily unavailable')
const periodClosed = () => fail(409, 'period_closed', 'The month is already closed')
const insufficient = (left: number, required: number) => fail(409, 'insufficient_points', 'Not enough points remain this week', { remaining: left, required })

/** The JSON object body, else undefined. */
export async function readJson(request: Request): Promise<Record<string, unknown> | undefined> {
  try {
    const parsed: unknown = await request.json()
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : undefined
  } catch {
    return undefined
  }
}

const PERIOD_KEY = /^\d{4}-W(0[1-9]|[1-4]\d|5[0-3])$/
const MONTH_KEY = /^\d{4}-(0[1-9]|1[0-2])$/

function timeline(url: URL): Response {
  const params = url.searchParams
  const limitParam = params.get('limit')
  const limit = limitParam === null ? DEFAULT_LIMIT : Number(limitParam)
  if (!Number.isInteger(limit) || limit < 1 || limit > MAX_LIMIT) return invalid(`limit must be between 1 and ${MAX_LIMIT}`)
  const cursorParam = params.get('cursor')
  const cursor = cursorParam ? decodeCursor(cursorParam) : undefined
  if (cursorParam && !cursor) return invalid('cursor is not valid')
  const value = params.get('value')
  if (value && !VALUE_KEY.test(value)) return invalid('value is not a valid key')
  const period = params.get('period')
  if (period && !PERIOD_KEY.test(period)) return invalid('period must look like 2026-W43')
  const month = params.get('month')
  if (month && !MONTH_KEY.test(month)) return invalid('month must look like 2026-10')
  const member = params.get('member')
  const sent = params.get('sent')
  const receivedBy = params.get('received')
  const q = params.get('q') ?? ''
  if (Array.from(q).length > 100) return invalid('q must be at most 100 characters')
  const needle = q.toLowerCase()
  const matching = live()
    .filter(
      (post) =>
        (!member || post.senderId === member || post.recipientId === member) &&
        (!sent || post.senderId === sent) &&
        (!receivedBy || post.recipientId === receivedBy) &&
        (!value || post.values.includes(value)) &&
        (!period || post.periodKey === period) &&
        (!month || post.monthKey === month) &&
        (!needle || [post.message, storedMember(post.senderId).displayName, storedMember(post.recipientId).displayName].some((text) => text.toLowerCase().includes(needle))) &&
        (!cursor || post.createdAt < cursor.createdAt || (post.createdAt === cursor.createdAt && post.id < cursor.id))
    )
    .sort(timelineOrder)
  const page = matching.slice(0, limit)
  const last = page.at(-1)
  return json({ items: page.map(postJson), nextCursor: matching.length > limit && last ? Buffer.from(`${last.createdAt}|${last.id}`, 'utf8').toString('base64url') : null })
}

function inboxItems(memberId: string): InboxItem[] {
  const seenAt = state.inboxSeenAt[memberId]
  const items: InboxItem[] = []
  for (const post of live()) {
    if (post.recipientId === memberId) items.push({
      id: post.id, kind: 'post', post: postJson(post), actor: memberJson(post.senderId),
      createdAt: post.createdAt, unread: !seenAt || post.createdAt > seenAt,
    })
    if (post.senderId !== memberId && post.recipientId !== memberId) continue
    for (const clap of state.claps) {
      if (clap.postId !== post.id || clap.memberId === memberId) continue
      items.push({
        id: clap.id, kind: 'clap', post: postJson(post), actor: memberJson(clap.memberId),
        createdAt: clap.createdAt, unread: !seenAt || clap.createdAt > seenAt,
      })
    }
  }
  return items.sort(timelineOrder)
}

function inbox(url: URL, memberId: string): Response {
  const limit = Number(url.searchParams.get('limit') ?? DEFAULT_LIMIT)
  if (!Number.isInteger(limit) || limit < 1 || limit > MAX_LIMIT) return invalid(`limit must be between 1 and ${MAX_LIMIT}`)
  const rawCursor = url.searchParams.get('cursor')
  const cursor = rawCursor ? decodeCursor(rawCursor) : undefined
  if (rawCursor && !cursor) return invalid('cursor is not valid')
  const matching = inboxItems(memberId).filter((item) => !cursor || item.createdAt < cursor.createdAt || (item.createdAt === cursor.createdAt && item.id < cursor.id))
  const items = matching.slice(0, limit)
  const last = items.at(-1)
  return json({
    items, seenAt: state.inboxSeenAt[memberId] ?? null,
    nextCursor: matching.length > limit && last ? Buffer.from(`${last.createdAt}|${last.id}`, 'utf8').toString('base64url') : null,
  })
}

/** `Idempotency-Key` (1–128 characters), else the 400 to answer. */
function idempotencyKey(request: Request): string | Response {
  const key = request.headers.get('Idempotency-Key') ?? ''
  return key.length >= 1 && key.length <= 128 ? key : invalid('Idempotency-Key header is required (1-128 characters)')
}

/** Upserts the Member of a Directory person by `personId` (name and email follow Directory). */
function upsertMember(person: MockPerson): MockMember {
  const existing = state.members.find((member) => member.personId === person.personId)
  if (existing) return Object.assign(existing, { displayName: person.displayName, email: person.email, subject: person.subject, avatarVersion: person.avatarVersion })
  const created: MockMember = { id: ulid(stamp(), state.seq++), personId: person.personId, subject: person.subject, displayName: person.displayName, email: person.email, avatarVersion: person.avatarVersion }
  state.members.push(created)
  return created
}

async function createPost(request: Request, callerId: string): Promise<Response> {
  const key = idempotencyKey(request)
  if (key instanceof Response) return key
  const replay = state.idempotency[`${callerId}|${key}`]
  if (replay) {
    const post = state.posts.find((candidate) => candidate.id === replay.postId)
    if (post) return json(postJson(post), 201)
  }
  const input = await readJson(request)
  if (!input) return invalid('A JSON object is required')
  const { recipientPersonId, points, message, values = [] } = input
  if (typeof recipientPersonId !== 'string' || recipientPersonId.length < 1 || recipientPersonId.length > 128) return invalid('recipientPersonId is required')
  const now = mockNow()
  const period = periodFor(now)
  if (typeof points !== 'number' || !Number.isInteger(points) || points < 1 || points > period.maxPointsPerPost) {
    return invalid(`points must be an integer between 1 and ${period.maxPointsPerPost}`)
  }
  if (typeof message !== 'string') return invalid('message is required')
  const problem = messageProblem(message)
  if (problem) return invalid({ messageRequired: 'message is required', messageTooLong: 'message must be at most 500 characters', messageControl: 'message must not contain control characters' }[problem])
  if (!Array.isArray(values) || values.length > MAX_VALUES || values.some((value) => typeof value !== 'string' || !VALUE_KEY.test(value)) || new Set(values).size !== values.length) {
    return invalid(`values must be at most ${MAX_VALUES} distinct keys`)
  }
  // Directory re-resolves the recipient; without it only an already-known Member can be sent to.
  let recipient: MockMember | undefined
  if (state.directoryUp) {
    const person = state.people.find((candidate) => candidate.personId === recipientPersonId)
    if (!person) return fail(404, 'recipient_not_found', 'The recipient is not in Directory')
    if (person.personId === storedMember(callerId).personId) return fail(400, 'self_target', 'You cannot send points to yourself')
    recipient = upsertMember(person)
  } else {
    recipient = state.members.find((member) => member.personId === recipientPersonId)
    if (!recipient) return directoryUnavailable()
    if (recipient.id === callerId) return fail(400, 'self_target', 'You cannot send points to yourself')
  }
  if ((values as string[]).some((value) => !state.program.values.some((known) => known.key === value))) return fail(400, 'unknown_value', 'A value is not defined by the program')
  if (state.statements[monthKeyAt(now, state.program.timeZone)]) return periodClosed()
  const left = remaining(callerId, period)
  if (points > left) return insufficient(left, points)
  const post = writePost(callerId, recipient.id, points, message.trim(), values as string[], stamp())
  state.idempotency[`${callerId}|${key}`] = { postId: post.id, clapId: null }
  return json(postJson(post), 201)
}

function createClap(request: Request, post: MockPost, callerId: string): Response {
  const key = idempotencyKey(request)
  if (key instanceof Response) return key
  const replay = state.idempotency[`${callerId}|${key}`]
  const replayed = replay?.clapId ? state.claps.find((clap) => clap.id === replay.clapId) : undefined
  if (replayed) return json({ clap: clapJson(replayed), post: postJson(post) }, 201)
  if (post.senderId === callerId || post.recipientId === callerId) return fail(403, 'own_post', 'You cannot clap for a post you sent or received')
  const now = mockNow()
  if (state.statements[post.monthKey] || state.statements[monthKeyAt(now, state.program.timeZone)]) return periodClosed()
  const left = remaining(callerId, periodFor(now))
  if (left < CLAP_COST) return insufficient(left, CLAP_COST)
  const clap = writeClap(post, callerId, stamp())
  state.idempotency[`${callerId}|${key}`] = { postId: post.id, clapId: clap.id }
  return json({ clap: clapJson(clap), post: postJson(post) }, 201)
}

/** Hides the post; while its month is open every ledger row it caused is reversed in its own week and month. */
function deletePost(post: MockPost, callerId: string): Response {
  const at = stamp()
  post.deletedAt = new Date(at).toISOString()
  post.deletedBy = callerId
  post.reversed = !state.statements[post.monthKey]
  if (post.reversed) {
    for (const entry of state.ledger.filter((row) => row.postId === post.id && row.kind !== 'reversal')) {
      append({ ...entry, delta: -entry.delta, kind: 'reversal', reversesId: entry.id, createdAt: post.deletedAt })
    }
  }
  return new Response(null, { status: 204, headers: { 'Cache-Control': 'no-store' } })
}

/** Directory people other than the caller, in name order (Directory's `/people` order). */
function othersByName(callerId: string): MockPerson[] {
  const self = storedMember(callerId).personId
  return state.people.filter((person) => person.personId !== self).sort((a, b) => a.displayName.localeCompare(b.displayName, 'ja'))
}

/** Directory's effective membership: a direct member of `group` or of any group under its path. */
function inGroup(person: MockPerson, group: RecipientGroup): boolean {
  return person.groups.some((id) => {
    const path = state.groups.find((candidate) => candidate.id === id)?.path
    return path !== undefined && (path === group.path || path.startsWith(`${group.path}/`))
  })
}

/** `groups`: the caller's teams the person is in (team order); the server knows no other affiliation. */
function candidateJson(person: Pick<MockPerson, 'personId' | 'displayName' | 'email' | 'subject' | 'avatarVersion'>, teams: RecipientGroup[] = []): Candidate {
  const known = state.people.find((candidate) => candidate.personId === person.personId)
  return {
    personId: person.personId,
    displayName: person.displayName,
    email: person.email,
    pending: person.subject === null,
    avatarUrl: avatarUrl(person),
    groups: known ? teams.filter((group) => inGroup(known, group)).map((group) => ({ ...group })) : [],
  }
}

function search(url: URL, callerId: string): Response {
  const q = url.searchParams.get('q') ?? ''
  if (Array.from(q).length > 64) return invalid('q must be at most 64 characters')
  const limitParam = url.searchParams.get('limit')
  const limit = limitParam === null ? 10 : Number(limitParam)
  if (!Number.isInteger(limit) || limit < 1 || limit > 20) return invalid('limit must be between 1 and 20')
  if (!state.directoryUp) return directoryUnavailable()
  const needle = q.toLowerCase()
  const items: Candidate[] = othersByName(callerId)
    .filter((person) => person.displayName.toLowerCase().includes(needle) || person.email.toLowerCase().includes(needle))
    .slice(0, limit)
    .map((person) => candidateJson(person))
  return json({ items })
}

const RECENT_LIMIT = 8
const TEAM_LIMIT = 5
const PEOPLE_LIMIT = 100

/**
 * `GET /recipients`: the people the caller last sent to (newest first, distinct), their direct groups
 * deepest first with each group's effective members, and everyone, all in name order without the
 * caller. Directory down leaves only `recent`, from the stored Members.
 */
function recipients(callerId: string): Response {
  const self = storedMember(callerId).personId
  const recentIds: string[] = []
  for (const post of [...state.posts].sort((a, b) => b.createdAt.localeCompare(a.createdAt) || b.id.localeCompare(a.id))) {
    if (post.senderId !== callerId || post.deletedAt !== null || recentIds.includes(post.recipientId)) continue
    if (storedMember(post.recipientId).personId === self) continue
    recentIds.push(post.recipientId)
    if (recentIds.length === RECENT_LIMIT) break
  }
  const recentMembers = recentIds.map(storedMember)
  if (!state.directoryUp) {
    const body: Recipients = {
      me: { personId: self, groups: [] },
      recent: recentMembers.map((member) => candidateJson(member)),
      teams: [],
      everyone: { total: 0, items: [] },
      directoryAvailable: false,
    }
    return json(body)
  }
  const caller = state.people.find((person) => person.personId === self)
  // Deepest path first; membership order breaks ties (Array.sort is stable).
  const direct = (caller?.groups ?? [])
    .map((id) => state.groups.find((group) => group.id === id))
    .filter((group): group is RecipientGroup => group !== undefined)
    .sort((a, b) => b.path.split('/').length - a.path.split('/').length)
  const teams = direct.slice(0, TEAM_LIMIT)
  const others = othersByName(callerId)
  const body: Recipients = {
    me: { personId: self, groups: direct.map((group) => ({ ...group })) },
    recent: recentMembers.map((member) => candidateJson(state.people.find((person) => person.personId === member.personId) ?? member, teams)),
    teams: teams.map((group) => ({
      group: { ...group },
      members: others
        .filter((person) => inGroup(person, group))
        .slice(0, PEOPLE_LIMIT)
        .map((person) => candidateJson(person, teams)),
    })),
    everyone: { total: others.length, items: others.slice(0, PEOPLE_LIMIT).map((person) => candidateJson(person, teams)) },
    directoryAvailable: true,
  }
  return json(body)
}

/** Handles `GET /config` and `/api/peerbonus/v1/*`; undefined for any other path. */
export async function peerbonusRoutes(request: Request, url: URL): Promise<Response | undefined> {
  if (request.method === 'GET' && url.pathname === '/config') return json(mockConfig)
  if (!url.pathname.startsWith(`${BASE}/`)) return undefined
  const route = `${request.method} ${url.pathname.slice(BASE.length)}`
  let match: RegExpMatchArray | null

  if (route === 'POST /__mock/reset') {
    seed()
    return json({ reset: true })
  }
  if (route === 'POST /__mock/caller') {
    const input = await readJson(request)
    if (typeof input?.memberId !== 'string' || !state.members.some((member) => member.id === input.memberId)) return invalid('memberId must name a member')
    state.callerId = input.memberId
    state.admin = input.admin === true
    return json({ callerId: state.callerId, admin: state.admin })
  }
  if (route === 'POST /__mock/directory') {
    const input = await readJson(request)
    state.directoryUp = input?.up !== false
    return json({ up: state.directoryUp })
  }

  for (const handler of extraRoutes) {
    const response = await handler(request, url)
    if (response) return response
  }

  const caller = state.callerId
  const now = mockNow()

  if ((match = route.match(/^GET \/avatars\/([^/]+)$/))) {
    if (!state.directoryUp) return directoryUnavailable()
    const personId = decodeURIComponent(match[1]!)
    const person = state.people.find((candidate) => candidate.personId === personId)
    if (!person?.avatarVersion || !person.avatarColor) return notFound()
    return new Response(mockAvatarPng(person.avatarColor), {
      headers: { 'Content-Type': 'image/png', 'Cache-Control': 'private, max-age=86400' },
    })
  }

  if (route === 'GET /inbox') return inbox(url, caller)
  if (route === 'POST /inbox/seen') {
    const seenAt = new Date(Math.max(now, state.lastStamp, Date.parse(state.inboxSeenAt[caller] ?? '') || 0)).toISOString()
    state.inboxSeenAt[caller] = seenAt
    return json({ seenAt, unread: 0 })
  }
  if (route === 'GET /me') {
    const me: Me = {
      member: memberJson(caller),
      roles: { peerbonusAdmin: state.admin },
      program: programJson(),
      period: periodFor(now),
      stats: myStats(caller, now),
      inbox: { unread: inboxItems(caller).filter((item) => item.unread).length },
      directory: { linked: state.directoryUp, publicUrl: DIRECTORY_PUBLIC_URL || null },
    }
    return json(me)
  }
  if (route === 'GET /program') return json(programJson())
  if (route === 'GET /members') return search(url, caller)
  if (route === 'GET /recipients') return recipients(caller)
  if ((match = route.match(/^GET \/members\/([^/]+)$/))) {
    const memberId = decodeURIComponent(match[1]!)
    if (!state.members.some((member) => member.id === memberId)) return notFound()
    const person = state.people.find((candidate) => candidate.personId === storedMember(memberId).personId)
    const groups = state.directoryUp ? state.groups.filter((group) => person?.groups.includes(group.id)) : []
    return json({ member: memberJson(memberId), groups, stats: memberStats(memberId, now) })
  }
  if (route === 'GET /posts') return timeline(url)
  if (route === 'POST /posts') return createPost(request, caller)

  if ((match = route.match(/^(GET|DELETE|POST) \/posts\/([^/]+)(\/claps)?$/))) {
    const [, method, rawId, claps] = match
    const post = state.posts.find((candidate) => candidate.id === decodeURIComponent(rawId!) && candidate.deletedAt === null)
    if (method === 'DELETE' && !claps && !state.admin) return forbidden()
    if (!post) return notFound()
    if (method === 'GET' && !claps) return json(postJson(post))
    if (method === 'DELETE' && !claps) return deletePost(post, caller)
    if (method === 'GET' && claps) {
      const items = state.claps
        .filter((clap) => clap.postId === post.id)
        .sort((a, b) => b.createdAt.localeCompare(a.createdAt) || b.id.localeCompare(a.id))
        .slice(0, MAX_CLAPS)
        .map(clapJson)
      return json({ items })
    }
    if (method === 'POST' && claps) return createClap(request, post, caller)
  }

  return notFound()
}
