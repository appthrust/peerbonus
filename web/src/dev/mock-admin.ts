// Dev-only admin half of the peerbonus contract (api.md §3 `/admin/*`),
// mounted into ./mock-peerbonus through `registerMockRoutes` (importing this
// module is enough) and working on its live state:
//   - Program: PATCH validates like domain.ValidateProgramPatch; the time zone
//     locks once the ledger has a row (409 `timezone_locked`).
//   - Statements: months from the oldest ledger month to the current one; a
//     month closes only after it ended in the program time zone (by the mock
//     clock, `setMockClock`), oldest first (409 `month_not_ended` /
//     `previous_month_open`), idempotently. Closing freezes the current rate
//     and currency; `directoryRefreshed` follows the mock Directory
//     (`POST /__mock/directory {up}`).
//   - CSV (closed months only, else 409 `statement_open`): UTF-8 with BOM and
//     the spec §3.5 header.
// v2 read models: Directory-wide participation (members-only when degraded),
// month insights with zero-filled ISO weeks, and statement close checklists.
//   - Slack (`/admin/slack*`, design slack-integration §5): starts `configured`
//     (bot token set, workspace not verified, no signing secret, no inbound).
//     Verify fixes the token's workspace (`Acme`); PATCH validates and
//     persists; a test message to a channel the bot is in records a sent
//     delivery (an announce of the newest post), one the bot is not in is 409
//     `slack_bot_not_in_channel`. `POST /__mock/slack {botToken?, signingSecret?,
//     team?: {teamId, teamName}, inbound?, invite?: channelId}` changes the
//     environment: a different token team turns a connected integration into
//     `team_changed`. The Slack state lives with the mock state, so reset
//     starts it over.
//   - Network map (`GET /admin/network`, design "v3: network map"): the live
//     posts and claps plus a fixed, deterministic "rest of the organisation"
//     (31 more Directory people in the same groups, about 75 days of posts and
//     claps anchored at the first read after a reset): a hub, a person the
//     month's thanks pile onto, one whose activity dropped two weeks ago, four
//     with one or two partners (their posts get no claps) and six with no
//     activity (two never signed in). Facts and signals follow the server's
//     rules (domain.NetworkThresholds); the extra people's member pages are
//     not mocked.
import type {
  Insights,
  Network,
  NetworkEdge,
  NetworkNode,
  NetworkSignal,
  NetworkThresholds,
  Participation,
  ProgramAdmin,
  RecipientGroup,
  SlackAdmin,
  SlackChannel,
  SlackDelivery,
  SlackDeliveryState,
  SlackInbound,
  SlackSettings,
  SlackState,
  Statement,
  StatementLine,
  StatementSummary,
  Value,
} from '../api/peerbonus'
import { isKnownTimeZone, MAX_VALUES, MAX_WEEKLY_ALLOWANCE } from '../features/admin/program-form'
import { nextMonth } from '../features/admin/statement-state'
import { isMonthKey, monthEnded, monthKeyAt, periodAt } from '../model/period'
import {
  avatarUrl,
  fail,
  forbidden,
  invalid,
  isAdmin,
  json,
  memberJson,
  MOCK_GROUPS,
  mockNow,
  mockState,
  programJson,
  readJson,
  received,
  registerMockRoutes,
  topPartners,
  valueCounts,
  type MockPerson,
  type MockState,
  type MockStatement,
} from './mock-peerbonus'

const BASE = '/api/peerbonus/v1'
const VALUE_KEY = /^[a-z0-9-]{1,32}$/
const DECIMAL = /^\d+(\.\d+)?$/
const CURRENCY = /^[A-Z]{3}$/
/** Amounts carry at least two fraction digits ("1234.00"), more when the rate has them. */
const MIN_AMOUNT_SCALE = 2

export const CSV_HEADER = 'month,person_id,email,display_name,pending,points,unit_value,currency,amount'

/** `points × rate` as an exact decimal string with `scale` fraction digits. */
function amountUnits(points: number, rate: string, scale: number): bigint {
  const [whole, fraction = ''] = rate.split('.')
  return BigInt(points) * BigInt(`${whole}${fraction.padEnd(scale, '0')}`)
}

function decimalText(units: bigint, scale: number): string {
  const digits = units.toString().padStart(scale + 1, '0')
  return `${digits.slice(0, -scale)}.${digits.slice(-scale)}`
}

const scaleOf = (rate: string) => Math.max(MIN_AMOUNT_SCALE, rate.split('.')[1]?.length ?? 0)

function programAdminJson(): ProgramAdmin {
  const state = mockState()
  return { ...programJson(), pointUnitValue: state.program.pointUnitValue, currency: state.program.currency }
}

/** Members who received points in `month`, most first, at `rate`; zero-point members are left out. */
function linesFor(month: string, rate: string): MockStatement['lines'] {
  const scale = scaleOf(rate)
  return mockState()
    .members.map((member) => ({ member, points: received(member.id, month) }))
    .filter((line) => line.points > 0)
    .sort((a, b) => b.points - a.points || a.member.displayName.localeCompare(b.member.displayName, 'ja') || a.member.id.localeCompare(b.member.id))
    .map(({ member, points }) => ({ memberId: member.id, points, amount: decimalText(amountUnits(points, rate, scale), scale), pending: member.subject === null }))
}

/** A closed month as frozen, else the running estimate at the current rate. */
function statementJson(month: string): Statement {
  const state = mockState()
  const closed = state.statements[month]
  const rate = closed?.pointUnitValue ?? state.program.pointUnitValue
  const scale = scaleOf(rate)
  const stored = closed?.lines ?? linesFor(month, rate)
  const lines: StatementLine[] = stored.map((line) => ({ member: { ...memberJson(line.memberId), pending: line.pending }, points: line.points, amount: line.amount }))
  const previousOpenMonth = closed ? null : state.ledger.map((entry) => entry.monthKey).filter((key) => key < month && !state.statements[key]).sort()[0] ?? null
  const closeBlockers: Statement['closeBlockers'] = []
  if (!closed && !monthEnded(month, mockNow(), state.program.timeZone)) closeBlockers.push('month_not_ended')
  if (previousOpenMonth) closeBlockers.push('previous_month_open')
  return {
    month,
    closed: Boolean(closed),
    closedAt: closed?.closedAt ?? null,
    closedBy: closed ? memberJson(closed.closedBy) : null,
    pointUnitValue: rate,
    currency: closed?.currency ?? state.program.currency,
    totalPoints: stored.reduce((sum, line) => sum + line.points, 0),
    totalAmount: decimalText(
      stored.reduce((sum, line) => sum + amountUnits(line.points, rate, scale), 0n),
      scale
    ),
    memberCount: stored.length,
    directoryRefreshed: closed?.directoryRefreshed ?? false,
    lines,
    closeBlockers,
    previousOpenMonth,
  }
}

/** Oldest ledger month through the current month, newest first. */
function statementSummaries(): StatementSummary[] {
  const state = mockState()
  const current = monthKeyAt(mockNow(), state.program.timeZone)
  let oldest = current
  for (const entry of state.ledger) if (entry.monthKey < oldest) oldest = entry.monthKey
  const items: StatementSummary[] = []
  for (let month = oldest; month <= current; month = nextMonth(month)) {
    const { closed, closedAt, totalPoints, totalAmount, memberCount } = statementJson(month)
    items.push({ month, closed, closedAt, totalPoints, totalAmount, memberCount })
  }
  return items.reverse()
}

function closeStatement(month: string): Response {
  const state = mockState()
  if (state.statements[month]) return json(statementJson(month))
  const now = mockNow()
  if (!monthEnded(month, now, state.program.timeZone)) return fail(409, 'month_not_ended', 'The month has not ended yet')
  if (state.ledger.some((entry) => entry.monthKey < month && !state.statements[entry.monthKey])) return fail(409, 'previous_month_open', 'An earlier month with ledger entries is still open')
  state.statements[month] = {
    month,
    closedAt: new Date(now).toISOString(),
    closedBy: state.callerId,
    pointUnitValue: state.program.pointUnitValue,
    currency: state.program.currency,
    directoryRefreshed: state.directoryUp,
    lines: linesFor(month, state.program.pointUnitValue),
  }
  return json(statementJson(month))
}

const csvField = (value: string) => (/[",\r\n]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value)

/** The payroll CSV of a closed month: BOM, the spec header, one row per line. */
export function statementCsv(month: string): string {
  const statement = statementJson(month)
  const rows = statement.lines.map((line) =>
    [month, line.member.personId, line.member.email, line.member.displayName, String(line.member.pending), String(line.points), statement.pointUnitValue, statement.currency, line.amount]
      .map(csvField)
      .join(',')
  )
  return `\uFEFF${[CSV_HEADER, ...rows].join('\n')}\n`
}

function adminMembers(month: string): Participation {
  const state = mockState()
  const posts = state.posts.filter((post) => post.deletedAt === null && post.monthKey === month)
  const liveIds = new Set(state.posts.filter((post) => post.deletedAt === null).map((post) => post.id))
  const directoryIds = new Set(state.directoryUp ? state.people.map((person) => person.personId) : [])
  const people = state.directoryUp ? [...state.people, ...state.members.filter((member) => !directoryIds.has(member.personId))] : state.members
  const items = people.map((person) => {
    const member = state.members.find((candidate) => candidate.personId === person.personId)
    const sent = posts.filter((post) => post.senderId === member?.id)
    return {
      personId: person.personId, displayName: person.displayName, email: person.email, pending: person.subject === null,
      avatarUrl: avatarUrl(person),
      member: member ? memberJson(member.id) : null,
      sentPosts: sent.length, sentPoints: sent.reduce((sum, post) => sum + post.points, 0),
      receivedPosts: posts.filter((post) => post.recipientId === member?.id).length,
      receivedPoints: member ? received(member.id, month) : 0,
      clapsGiven: state.claps.filter((clap) => clap.memberId === member?.id && clap.monthKey === month && liveIds.has(clap.postId)).length,
    }
  }).sort((a, b) => Number(directoryIds.has(b.personId)) - Number(directoryIds.has(a.personId)) || b.receivedPoints - a.receivedPoints || b.sentPoints - a.sentPoints || a.displayName.localeCompare(b.displayName, 'ja') || a.personId.localeCompare(b.personId))
  return {
    month, directoryAvailable: state.directoryUp, items,
    summary: {
      people: items.length,
      senders: items.filter((row) => row.sentPosts > 0).length,
      recipients: items.filter((row) => row.receivedPosts > 0).length,
      active: items.filter((row) => row.sentPosts > 0 || row.receivedPosts > 0 || row.clapsGiven > 0 || row.receivedPoints > 0).length,
    },
  }
}

function insights(month: string): Insights {
  const state = mockState()
  const posts = state.posts.filter((post) => post.deletedAt === null && post.monthKey === month)
  const liveIds = new Set(state.posts.filter((post) => post.deletedAt === null).map((post) => post.id))
  const claps = state.claps.filter((clap) => clap.monthKey === month && liveIds.has(clap.postId))
  const ledger = state.ledger.filter((entry) => entry.monthKey === month && entry.account === 'receive')
  const weeks: Insights['weeks'] = []
  const [year, number] = month.split('-').map(Number)
  // Probe every local calendar day via UTC noons. The extra days cover all IANA offsets.
  for (let day = 0; day <= 33; day++) {
    const at = Date.UTC(year!, number! - 1, day, 12)
    if (monthKeyAt(at, state.program.timeZone) !== month) continue
    const period = periodAt(at, state.program.timeZone)
    if (weeks.some((week) => week.key === period.key)) continue
    weeks.push({
      ...period, posts: posts.filter((post) => post.periodKey === period.key).length,
      points: ledger.filter((entry) => entry.periodKey === period.key).reduce((sum, entry) => sum + entry.delta, 0),
      claps: claps.filter((clap) => clap.periodKey === period.key).length,
    })
  }
  return {
    month, posts: posts.length, points: ledger.reduce((sum, entry) => sum + entry.delta, 0), claps: claps.length,
    senders: new Set(posts.map((post) => post.senderId)).size,
    recipients: new Set(posts.map((post) => post.recipientId)).size,
    values: valueCounts(posts), weeks,
    topRecipients: topPartners(posts, 'recipientId'), topSenders: topPartners(posts, 'senderId'),
  }
}

/** PATCH /admin/program: validation as domain.ValidateProgramPatch, then the time zone lock. */
async function updateProgram(request: Request): Promise<Response> {
  const state = mockState()
  const input = await readJson(request)
  if (!input) return invalid('A JSON object is required')
  const { timeZone, weeklyAllowance, maxPointsPerPost, pointUnitValue, currency, values } = input
  if (timeZone !== undefined && (typeof timeZone !== 'string' || !isKnownTimeZone(timeZone))) return invalid('timeZone must be an IANA time zone')
  if (weeklyAllowance !== undefined && !Number.isInteger(weeklyAllowance)) return invalid('weeklyAllowance must be an integer')
  if (maxPointsPerPost !== undefined && !Number.isInteger(maxPointsPerPost)) return invalid('maxPointsPerPost must be an integer')
  const allowance = (weeklyAllowance as number | undefined) ?? state.program.weeklyAllowance
  const max = (maxPointsPerPost as number | undefined) ?? state.program.maxPointsPerPost
  if (allowance < 0 || allowance > MAX_WEEKLY_ALLOWANCE) return invalid(`weeklyAllowance must be between 0 and ${MAX_WEEKLY_ALLOWANCE}`)
  if (max < 1 || max > allowance) return invalid('maxPointsPerPost must be between 1 and weeklyAllowance')
  if (pointUnitValue !== undefined && (typeof pointUnitValue !== 'string' || !DECIMAL.test(pointUnitValue))) return invalid('pointUnitValue must be a nonnegative decimal')
  if (currency !== undefined && (typeof currency !== 'string' || !CURRENCY.test(currency))) return invalid('currency must be a three-letter uppercase currency code')
  let nextValues: Value[] | undefined
  if (values !== undefined) {
    if (!Array.isArray(values) || values.length > MAX_VALUES) return invalid(`values must be a list of at most ${MAX_VALUES}`)
    nextValues = []
    for (const value of values as unknown[]) {
      const row = value as Record<string, unknown> | null
      const description = row?.description ?? null
      if (typeof row?.key !== 'string' || !VALUE_KEY.test(row.key) || typeof row.label !== 'string' || row.label.trim() === '' || (description !== null && typeof description !== 'string')) {
        return invalid('each value needs a valid key and nonempty label')
      }
      if (nextValues.some((previous) => previous.key === row.key)) return invalid('values must not contain duplicate keys')
      nextValues.push({ key: row.key, label: row.label, description: description as string | null })
    }
  }
  if (typeof timeZone === 'string' && timeZone !== state.program.timeZone) {
    if (state.ledger.length > 0) return fail(409, 'timezone_locked', 'The time zone cannot change once the ledger has entries')
    // Week rows computed in the old zone no longer line up; only empty ones can exist here.
    for (const key of Object.keys(state.periods)) if (!state.ledger.some((entry) => entry.periodKey === key)) delete state.periods[key]
    state.program.timeZone = timeZone
  }
  state.program.weeklyAllowance = allowance
  state.program.maxPointsPerPost = max
  if (typeof pointUnitValue === 'string') state.program.pointUnitValue = pointUnitValue
  if (typeof currency === 'string') state.program.currency = currency
  if (nextValues) state.program.values = nextValues
  // Strictly increasing, so every save is a new version even on a pinned mock clock.
  state.program.updatedAt = new Date(Math.max(mockNow(), Date.parse(state.program.updatedAt) + 1)).toISOString()
  state.program.updatedBy = state.callerId
  return json(programAdminJson())
}

/** The workspace the mock's bot token belongs to until `POST /__mock/slack {team}` changes it. */
export const MOCK_SLACK_TEAM = { teamId: 'T0MOCKACME', teamName: 'Acme', botUserId: 'U0PEERBONUS' }

/** Public channels (`conversations.list` order); the bot was invited to #general and #kudos. */
export const MOCK_SLACK_CHANNELS: readonly SlackChannel[] = [
  { id: 'C0GENERAL', name: 'general', isMember: true },
  { id: 'C0KUDOS', name: 'kudos', isMember: true },
  { id: 'C0RANDOM', name: 'random', isMember: false },
  { id: 'C0ANNOUNCE', name: 'announcements', isMember: false },
  { id: 'C0DEV', name: 'dev', isMember: false },
  { id: 'C0DESIGN', name: 'design', isMember: false },
  { id: 'C0SALES', name: 'sales', isMember: false },
  { id: 'C0SALESJP', name: 'sales-jp', isMember: false },
]

const CHANNEL_LIMIT = 20
const DELIVERY_LIMIT = 50
const DAY_MS = 24 * 60 * 60 * 1000
const DELIVERY_STATES: readonly SlackDeliveryState[] = ['pending', 'sent', 'skipped', 'failed']

type MockSlack = {
  botToken: boolean
  signingSecret: boolean
  /** What `auth.test` answers for the current token. */
  token: { teamId: string; teamName: string; botUserId: string }
  workspace: SlackAdmin['workspace']
  settings: SlackSettings
  inbound: SlackInbound | null
  lastError: SlackAdmin['lastError']
  channels: SlackChannel[]
  deliveries: SlackDelivery[]
  seq: number
}

const slackByState = new WeakMap<MockState, MockSlack>()

/** The Slack half of the live mock state, created with it (so `__mock/reset` starts it over). */
function mockSlack(): MockSlack {
  const state = mockState()
  let slack = slackByState.get(state)
  if (!slack) {
    slack = {
      botToken: true,
      signingSecret: false,
      token: { ...MOCK_SLACK_TEAM },
      workspace: null,
      settings: { announceChannel: null, announcePosts: false, dmRecipientOnPost: true, giveFromSlack: false, locale: 'ja' },
      inbound: null,
      lastError: null,
      channels: MOCK_SLACK_CHANNELS.map((channel) => ({ ...channel })),
      deliveries: [],
      seq: 1,
    }
    slackByState.set(state, slack)
  }
  return slack
}

/** As the server derives it: no token, not verified, the token's team differs from the fixed one, else connected. */
function slackState(slack: MockSlack): SlackState {
  if (!slack.botToken) return 'unconfigured'
  if (!slack.workspace) return 'configured'
  return slack.workspace.teamId === slack.token.teamId ? 'connected' : 'team_changed'
}

function slackJson(): SlackAdmin {
  const slack = mockSlack()
  const since = mockNow() - DAY_MS
  return {
    credential: { botToken: slack.botToken, signingSecret: slack.signingSecret },
    state: slackState(slack),
    workspace: slack.workspace && { ...slack.workspace },
    settings: { ...slack.settings, announceChannel: slack.settings.announceChannel && { ...slack.settings.announceChannel } },
    inbound: slack.inbound && { ...slack.inbound },
    lastError: slack.lastError && { ...slack.lastError },
    deliveries: {
      pending: slack.deliveries.filter((row) => row.state === 'pending').length,
      failed24h: slack.deliveries.filter((row) => row.state === 'failed' && Date.parse(row.createdAt) >= since).length,
      sent24h: slack.deliveries.filter((row) => row.state === 'sent' && row.sentAt !== null && Date.parse(row.sentAt) >= since).length,
    },
  }
}

const slackNotConnected = () => fail(409, 'slack_not_connected', 'Slack is not connected; verify the workspace first')

/** PATCH /admin/slack: types, a known channel, then the state and secret gates; applies everything or nothing. */
async function updateSlack(request: Request): Promise<Response> {
  const slack = mockSlack()
  const input = await readJson(request)
  if (!input) return invalid('A JSON object is required')
  const { announceChannelId, announcePosts, dmRecipientOnPost, giveFromSlack, locale } = input
  for (const flag of [announcePosts, dmRecipientOnPost, giveFromSlack]) if (flag !== undefined && typeof flag !== 'boolean') return invalid('flags must be booleans')
  if (locale !== undefined && locale !== 'ja' && locale !== 'en') return invalid('locale must be ja or en')
  if (announceChannelId !== undefined && announceChannelId !== null && typeof announceChannelId !== 'string') return invalid('announceChannelId must be a string or null')
  if (slackState(slack) !== 'connected') return slackNotConnected()
  let channel = slack.settings.announceChannel
  if (typeof announceChannelId === 'string') {
    const found = slack.channels.find((candidate) => candidate.id === announceChannelId)
    if (!found) return invalid('announceChannelId must name a public channel')
    channel = { id: found.id, name: found.name }
  } else if (announceChannelId === null) {
    channel = null
  }
  const next: SlackSettings = {
    announceChannel: channel,
    announcePosts: (announcePosts as boolean | undefined) ?? slack.settings.announcePosts,
    dmRecipientOnPost: (dmRecipientOnPost as boolean | undefined) ?? slack.settings.dmRecipientOnPost,
    giveFromSlack: (giveFromSlack as boolean | undefined) ?? slack.settings.giveFromSlack,
    locale: (locale as SlackSettings['locale'] | undefined) ?? slack.settings.locale,
  }
  if (next.announcePosts && !next.announceChannel) return invalid('announcePosts requires an announce channel')
  if (giveFromSlack === true && !slack.signingSecret) return fail(409, 'slack_signing_secret_missing', 'Set the Slack signing secret before giving from Slack')
  slack.settings = next
  return json(slackJson())
}

/** POST /admin/slack/verify: fixes the token's workspace; another workspace needs `force`, which also drops the channel. */
async function verifySlack(request: Request): Promise<Response> {
  const slack = mockSlack()
  const force = (await readJson(request))?.force === true
  if (!slack.botToken) return fail(409, 'slack_not_configured', 'No Slack bot token is configured')
  const verifiedAt = new Date(mockNow()).toISOString()
  if (slack.workspace && slack.workspace.teamId !== slack.token.teamId) {
    if (!force) {
      return fail(409, 'slack_team_changed', 'The bot token belongs to another Slack workspace', {
        detail: { current: { teamId: slack.workspace.teamId, teamName: slack.workspace.teamName }, token: { teamId: slack.token.teamId, teamName: slack.token.teamName } },
      })
    }
    slack.settings = { ...slack.settings, announceChannel: null, announcePosts: false }
  }
  slack.workspace = { ...slack.token, verifiedAt }
  slack.lastError = null
  return json(slackJson())
}

/** POST /admin/slack/test: posts to the saved channel; records it as a sent announce of the newest post. */
function testSlack(): Response {
  const slack = mockSlack()
  if (slackState(slack) !== 'connected') return slackNotConnected()
  const target = slack.settings.announceChannel
  if (!target) return invalid('Choose an announce channel first')
  if (!slack.channels.find((channel) => channel.id === target.id)?.isMember) {
    return fail(409, 'slack_bot_not_in_channel', 'Invite the bot to the channel with /invite @Peer Bonus')
  }
  const newest = mockState()
    .posts.filter((post) => post.deletedAt === null)
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt))[0]
  const at = new Date(mockNow()).toISOString()
  const ts = `${Math.floor(mockNow() / 1000)}.${String(slack.seq).padStart(6, '0')}`
  if (newest) {
    slack.deliveries.unshift({ id: slack.seq++, kind: 'post_announce', postId: newest.id, state: 'sent', attempts: 1, lastError: null, createdAt: at, sentAt: at })
  }
  return json({ ok: true, ts })
}

function slackChannels(url: URL): Response {
  const slack = mockSlack()
  if (!slack.botToken) return fail(409, 'slack_not_configured', 'No Slack bot token is configured')
  const limit = Number(url.searchParams.get('limit') ?? CHANNEL_LIMIT)
  if (!Number.isInteger(limit) || limit < 1 || limit > CHANNEL_LIMIT) return invalid(`limit must be between 1 and ${CHANNEL_LIMIT}`)
  const q = (url.searchParams.get('q') ?? '').trim().replace(/^#/, '').toLowerCase()
  return json({ items: slack.channels.filter((channel) => channel.name.startsWith(q)).slice(0, limit) })
}

function slackDeliveries(url: URL): Response {
  const state = url.searchParams.get('state') ?? ''
  if (state !== '' && !DELIVERY_STATES.includes(state as SlackDeliveryState)) return invalid('state must be pending, sent, skipped or failed')
  const limit = Number(url.searchParams.get('limit') ?? DELIVERY_LIMIT)
  if (!Number.isInteger(limit) || limit < 1 || limit > DELIVERY_LIMIT) return invalid(`limit must be between 1 and ${DELIVERY_LIMIT}`)
  const rows = mockSlack().deliveries.filter((row) => state === '' || row.state === state)
  return json({ items: rows.slice(0, limit).map((row) => ({ ...row })) })
}

/** `POST /__mock/slack`: the environment the server would see (credentials, the token's workspace, the inbound endpoint, bot invites). */
async function slackControl(request: Request, url: URL): Promise<Response | undefined> {
  if (request.method !== 'POST' || url.pathname !== `${BASE}/__mock/slack`) return undefined
  const slack = mockSlack()
  const input = (await readJson(request)) ?? {}
  if (typeof input.botToken === 'boolean') slack.botToken = input.botToken
  if (typeof input.signingSecret === 'boolean') slack.signingSecret = input.signingSecret
  const team = input.team as { teamId?: unknown; teamName?: unknown } | undefined
  if (typeof team?.teamId === 'string' && typeof team.teamName === 'string') {
    slack.token = { teamId: team.teamId, teamName: team.teamName, botUserId: slack.token.botUserId }
    // The worker's auth.test notices the change and pauses deliveries (SL4).
    if (slackState(slack) === 'team_changed') slack.lastError = { code: 'team_changed', at: new Date(mockNow()).toISOString() }
  }
  if (input.inbound !== undefined) slack.inbound = input.inbound as SlackInbound | null
  if (typeof input.invite === 'string') {
    const channel = slack.channels.find((candidate) => candidate.id === input.invite)
    if (channel) channel.isMember = true
  }
  return json(slackJson())
}

// ---- Network map (`GET /admin/network`) ----

/** domain.DefaultNetworkThresholds. */
const NETWORK_THRESHOLDS: NetworkThresholds = { manyConnections: 8, fewConnections: 2, concentrationPosts: 5, concentrationShare: 0.2, declineRatio: 0.5, declineMinimum: 4 }
/** domain.DeclineWindow. */
const DECLINE_WINDOW_MS = 14 * DAY_MS
const NET_HOUR = 60 * 60 * 1000

type NetRole = 'hub' | 'star' | 'declining' | 'few' | 'isolated' | 'regular'
type NetPerson = MockPerson & { role: NetRole; memberId: string | null }
type NetPost = { id: string; sender: string; recipient: string; points: number; at: number }
type NetClap = { postId: string; clapper: string; at: number }
type NetLog = { people: NetPerson[]; posts: NetPost[]; claps: NetClap[] }

/** [number, name, direct group, role, never signed in]. */
const NET_COHORT: readonly (readonly [string, string, keyof typeof MOCK_GROUPS | null, NetRole, boolean?])[] = [
  ['01', '高橋 健一', 'teamB', 'hub'],
  ['02', '森 由美', 'ceatec', 'star'],
  ['03', '清水 拓也', 'teamA', 'declining'],
  ['04', '山本 直樹', 'teamA', 'regular'],
  ['05', '井上 彩', 'teamA', 'regular'],
  ['06', '木村 誠', 'teamA', 'regular'],
  ['07', '林 美穂', 'teamB', 'regular'],
  ['08', '斎藤 亮', 'teamB', 'regular'],
  ['09', '山口 千尋', 'teamB', 'regular'],
  ['10', '松本 悠斗', 'teamB', 'regular'],
  ['11', '池田 愛', 'ceatec', 'regular'],
  ['12', '橋本 大樹', 'ceatec', 'regular'],
  ['13', '阿部 真央', 'sales', 'regular'],
  ['14', '石川 翼', 'sales', 'regular'],
  ['15', '山下 結衣', 'teamA', 'regular'],
  ['16', '中島 蓮', 'teamB', 'regular'],
  ['17', '前田 陽菜', null, 'regular'],
  ['18', '藤田 颯', null, 'regular'],
  ['19', '小川 里奈', null, 'regular'],
  ['20', '岡田 智也', 'teamA', 'regular'],
  ['21', '後藤 美月', 'ceatec', 'regular'],
  ['22', '長谷川 樹', 'teamB', 'few'],
  ['23', '村上 彩花', 'teamA', 'few'],
  ['24', '近藤 航', null, 'few'],
  ['25', '石井 葵', 'sales', 'few'],
  ['26', '坂本 湊', 'teamB', 'isolated'],
  ['27', '遠藤 七海', 'teamA', 'isolated', true],
  ['28', '青木 陸', 'ceatec', 'isolated'],
  ['29', '藤井 杏', null, 'isolated', true],
  ['30', '西村 隼', 'sales', 'isolated'],
  ['31', '福田 楓', 'teamB', 'isolated'],
]
/** The only counterparts of the `few` people (by cohort number). */
const NET_FEW_PARTNERS: Record<string, string[]> = { '22': ['07'], '23': ['05'], '24': ['17', '18'], '25': ['13'] }
const NET_POINTS = [10, 20, 30, 50, 15, 40] as const

/** mulberry32: a small seeded PRNG, so the extra activity is the same after every reset. */
function seededRandom(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const netLogs = new WeakMap<MockState, NetLog>()

/** The extra people and their posts and claps, built once per mock state and anchored at the first read. */
function networkLog(): NetLog {
  const state = mockState()
  const cached = netLogs.get(state)
  if (cached) return cached
  const base = mockNow()
  const rand = seededRandom(0x5eed2026)
  const pick = <T,>(items: readonly T[]): T => items[Math.floor(rand() * items.length)]!
  const pid = (number: string) => `p-net-${number}`
  const people: NetPerson[] = NET_COHORT.map(([number, displayName, group, role, pending]) => ({
    personId: pid(number),
    subject: pending ? null : `sub-net-${number}`,
    displayName,
    email: `net${number}@example.com`,
    groups: group ? [MOCK_GROUPS[group].id] : [],
    avatarVersion: null,
    avatarColor: null,
    role,
    memberId: role === 'isolated' ? null : `01J9PBMEMBERNET0000000000${number}`,
  }))
  const groupsOf = new Map<string, string[]>([...state.people, ...people].map((person) => [person.personId, person.groups]))
  const sameGroup = (a: string, b: string) => (groupsOf.get(a) ?? []).some((group) => (groupsOf.get(b) ?? []).includes(group))
  const role = (r: NetRole) => people.filter((person) => person.role === r).map((person) => person.personId)
  const hub = role('hub')[0]!
  const star = role('star')[0]!
  const declining = role('declining')[0]!
  const realActive = state.members.filter((member) => member.subject !== null).map((member) => member.personId)
  const realPending = state.members.filter((member) => member.subject === null).map((member) => member.personId)
  const senders = [...role('regular'), star, ...realActive]
  const recipients = [...senders, ...realPending]
  const clappers = [...senders, hub]
  const posts: NetPost[] = []
  const claps: NetClap[] = []
  const span = 75 * DAY_MS
  const from = base - span
  const declineCut = base - 15 * DAY_MS
  const write = (sender: string, recipient: string, at: number) => {
    const post = { id: `net-post-${posts.length + 1}`, sender, recipient, points: pick(NET_POINTS), at }
    posts.push(post)
    return post
  }
  const clap = (post: NetPost, clapper: string) => claps.push({ postId: post.id, clapper, at: Math.min(post.at + (1 + rand() * 30) * NET_HOUR, base - 60_000) })

  const count = 140
  for (let index = 0; index < count; index++) {
    const at = from + ((index + rand()) * span) / count
    // Senders take turns (an even rhythm, so only the declining person's activity drops); the hub sends often.
    const sender = rand() < 0.14 ? hub : senders[(index * 11) % senders.length]!
    let recipient: string
    // Every other post thanks the same person (the month's concentration).
    if (sender !== star && index % 2 === 0) recipient = star
    else {
      const peers = recipients.filter((person) => person !== sender && sameGroup(person, sender))
      recipient = sender !== hub && peers.length > 0 && rand() < 0.65 ? pick(peers) : pick(recipients.filter((person) => person !== sender))
    }
    const post = write(sender, recipient, at)
    // One clapper from the recipient's team, mostly; the hub claps a quarter of the posts as well; some get none.
    if (rand() >= 0.12) {
      const local = clappers.filter((person) => person !== sender && person !== recipient && person !== hub && sameGroup(person, recipient))
      clap(post, pick(local.length > 0 && rand() < 0.85 ? local : clappers.filter((person) => person !== sender && person !== recipient)))
      if (sender !== hub && recipient !== hub && rand() < 0.25) clap(post, hub)
    }
    // The declining person still clapped their team until two weeks ago.
    if (at < declineCut && sameGroup(declining, recipient) && rand() < 0.5) clap(post, declining)
  }
  // Declining: a post to the team every three days until two weeks ago, then one.
  const team = recipients.filter((person) => sameGroup(person, declining))
  for (const daysAgo of [...Array.from({ length: 20 }, (_, index) => 73 - index * 3), 3]) {
    const post = write(declining, pick(team), base - daysAgo * DAY_MS + 3 * NET_HOUR)
    if (rand() < 0.7) clap(post, pick(clappers.filter((person) => person !== post.recipient && person !== hub)))
  }
  // Few: a post with each partner every nine days, alternating direction, never clapped.
  for (const [number, partners] of Object.entries(NET_FEW_PARTNERS)) {
    for (const [offset, partner] of partners.entries()) {
      for (let daysAgo = 70 - Number(number) - offset * 4, turn = 0; daysAgo > 0; daysAgo -= 9, turn++) {
        const [a, b] = turn % 2 === 0 ? [pid(number), pid(partner)] : [pid(partner), pid(number)]
        write(a, b, base - daysAgo * DAY_MS)
      }
    }
  }
  posts.sort((a, b) => a.at - b.at)
  const log = { people, posts, claps }
  netLogs.set(state, log)
  return log
}

/** The last instant of `month` in `timeZone` (the instant before the next month's first midnight). */
function monthLastInstant(month: string, timeZone: string): number {
  const [year, number] = month.split('-').map(Number) as [number, number]
  let lo = Date.UTC(year, number, 1) - 15 * NET_HOUR
  let hi = Date.UTC(year, number, 1) + 15 * NET_HOUR
  while (lo < hi) {
    const mid = Math.floor((lo + hi) / 2)
    if (monthEnded(month, mid, timeZone)) hi = mid
    else lo = mid + 1
  }
  return lo - 1
}

/** Directory's effective membership: a direct member of `group` or of a group under its path. */
function inNetworkGroup(person: Pick<MockPerson, 'groups'>, group: RecipientGroup, groups: readonly RecipientGroup[]): boolean {
  return person.groups.some((id) => {
    const path = groups.find((candidate) => candidate.id === id)?.path
    return path !== undefined && (path === group.path || path.startsWith(`${group.path}/`))
  })
}

type NetFacts = Omit<NetworkNode, 'personId' | 'memberId' | 'displayName' | 'email' | 'pending' | 'avatarUrl' | 'connections' | 'share' | 'decline' | 'signals'> & {
  counterparts: Set<string>
  recent: number
  previous: number
}

/** `GET /admin/network?month=&group=`: as internal/api/network.go answers it, over the live state plus the extra people. */
function network(month: string, groupId: string): Response {
  const state = mockState()
  const tz = state.program.timeZone
  const now = mockNow()
  const current = monthKeyAt(now, tz) === month
  const asOf = current ? now : monthLastInstant(month, tz)
  const group = state.directoryUp && groupId ? state.groups.find((candidate) => candidate.id === groupId) : undefined
  if (state.directoryUp && groupId && !group) return fail(404, 'group_not_found', 'Directory lists no such group')

  const log = networkLog()
  const personOf = new Map(state.members.map((member) => [member.id, member.personId]))
  const posts: NetPost[] = [
    ...state.posts
      .filter((post) => post.deletedAt === null)
      .map((post) => ({ id: post.id, sender: personOf.get(post.senderId)!, recipient: personOf.get(post.recipientId)!, points: post.points, at: Date.parse(post.createdAt) })),
    ...log.posts,
  ]
  const postById = new Map(posts.map((post) => [post.id, post]))
  const claps: NetClap[] = [
    ...state.claps.filter((clap) => postById.has(clap.postId)).map((clap) => ({ postId: clap.postId, clapper: personOf.get(clap.memberId)!, at: Date.parse(clap.createdAt) })),
    ...log.claps,
  ]
  const clapCount = new Map<string, number>()
  for (const clap of claps) clapCount.set(clap.postId, (clapCount.get(clap.postId) ?? 0) + 1)
  const windowPosts = posts.filter((post) => post.at <= asOf && monthKeyAt(post.at, tz) === month)
  const windowClaps = claps.filter((clap) => clap.at <= asOf && monthKeyAt(clap.at, tz) === month)

  const facts = new Map<string, NetFacts>()
  const factsOf = (personId: string): NetFacts => {
    let row = facts.get(personId)
    if (!row) {
      row = { sentPosts: 0, receivedPosts: 0, receivedPoints: 0, clapsGiven: 0, clapsReceived: 0, unclappedPosts: 0, counterparts: new Set(), recent: 0, previous: 0 }
      facts.set(personId, row)
    }
    return row
  }
  const edges = new Map<string, NetworkEdge>()
  const connect = (a: string, b: string, at: number, kind: { posts?: number; claps?: number; points?: number }) => {
    if (a === b) return
    const [from, to] = a < b ? [a, b] : [b, a]
    const key = `${from}|${to}`
    const edge = edges.get(key) ?? { from, to, posts: 0, claps: 0, points: 0, lastAt: new Date(at).toISOString() }
    edge.posts += kind.posts ?? 0
    edge.claps += kind.claps ?? 0
    edge.points += kind.points ?? 0
    if (at > Date.parse(edge.lastAt)) edge.lastAt = new Date(at).toISOString()
    edges.set(key, edge)
    factsOf(a).counterparts.add(b)
    factsOf(b).counterparts.add(a)
  }
  for (const post of windowPosts) {
    factsOf(post.sender).sentPosts++
    const recipient = factsOf(post.recipient)
    recipient.receivedPosts++
    // Live posts' points come from the ledger below (as the server reads them); the extra people have no ledger.
    if (post.id.startsWith('net-post-')) recipient.receivedPoints += post.points
    if (!clapCount.get(post.id)) recipient.unclappedPosts++
    connect(post.sender, post.recipient, post.at, { posts: 1, points: post.points })
  }
  for (const clap of windowClaps) {
    const post = postById.get(clap.postId)!
    factsOf(clap.clapper).clapsGiven++
    // As the server counts them: a clap on a post is received by its sender and its recipient.
    factsOf(post.sender).clapsReceived++
    factsOf(post.recipient).clapsReceived++
    connect(clap.clapper, post.sender, clap.at, { claps: 1 })
    connect(clap.clapper, post.recipient, clap.at, { claps: 1 })
  }
  // Own actions (sent posts + claps given) in the two decline windows ending at asOf.
  const tally = (personId: string, at: number) => {
    if (at > asOf || at <= asOf - 2 * DECLINE_WINDOW_MS) return
    const row = factsOf(personId)
    if (at > asOf - DECLINE_WINDOW_MS) row.recent++
    else row.previous++
  }
  for (const post of posts) tally(post.sender, post.at)
  for (const clap of claps) tally(clap.clapper, clap.at)

  // The roster: Directory people (plus members it no longer lists), or members only while it is down.
  const directoryIds = new Set([...state.people, ...log.people].map((person) => person.personId))
  const roster: (Pick<MockPerson, 'personId' | 'subject' | 'displayName' | 'email' | 'avatarVersion' | 'groups'> & { memberId: string | null })[] = state.directoryUp
    ? [
        ...state.people.map((person) => ({ ...person, memberId: state.members.find((member) => member.personId === person.personId)?.id ?? null })),
        ...log.people,
        ...state.members.filter((member) => !directoryIds.has(member.personId)).map((member) => ({ ...member, groups: [], memberId: member.id })),
      ]
    : [...state.members.map((member) => ({ ...member, groups: [], memberId: member.id })), ...log.people.filter((person) => person.memberId !== null)]
  const people = group ? roster.filter((person) => inNetworkGroup(person, group, state.groups)) : roster
  const t = NETWORK_THRESHOLDS
  const nodes: NetworkNode[] = people
    .map((person) => {
      const row = factsOf(person.personId)
      const connections = row.counterparts.size
      const share = windowPosts.length > 0 ? row.receivedPosts / windowPosts.length : 0
      const decline = row.previous >= t.declineMinimum ? { recent: row.recent, previous: row.previous } : null
      const signals: NetworkSignal[] = []
      if (connections === 0) signals.push('isolated')
      else if (connections <= t.fewConnections) signals.push('fewConnections')
      else if (connections >= t.manyConnections) signals.push('manyConnections')
      if (row.receivedPosts >= t.concentrationPosts && share >= t.concentrationShare) signals.push('concentrated')
      if (decline && decline.recent <= t.declineRatio * decline.previous) signals.push('declining')
      if (row.unclappedPosts > 0) signals.push('unclapped')
      return {
        personId: person.personId,
        memberId: person.memberId,
        displayName: person.displayName,
        email: person.email,
        pending: person.subject === null,
        avatarUrl: avatarUrl(person),
        connections,
        sentPosts: row.sentPosts,
        receivedPosts: row.receivedPosts,
        receivedPoints: row.receivedPoints + (person.memberId !== null && state.members.some((member) => member.id === person.memberId) ? received(person.memberId, month) : 0),
        clapsGiven: row.clapsGiven,
        clapsReceived: row.clapsReceived,
        unclappedPosts: row.unclappedPosts,
        share,
        decline,
        signals,
      }
    })
    .sort((a, b) => a.displayName.localeCompare(b.displayName, 'ja') || a.personId.localeCompare(b.personId))
  const shown = new Set(nodes.map((node) => node.personId))
  const body: Network = {
    month,
    asOf: new Date(asOf).toISOString(),
    recentSince: current ? periodAt(now, tz).startsAt : null,
    directoryAvailable: state.directoryUp,
    group: group ? { ...group } : null,
    groups: state.directoryUp ? [...state.groups].sort((a, b) => a.name.localeCompare(b.name, 'ja') || a.id.localeCompare(b.id)).map((item) => ({ ...item })) : [],
    thresholds: { ...t },
    summary: {
      people: nodes.length,
      connected: nodes.filter((node) => node.connections > 0).length,
      isolated: nodes.filter((node) => node.connections === 0).length,
      posts: windowPosts.length,
      claps: windowClaps.length,
    },
    nodes,
    edges: [...edges.values()].filter((edge) => shown.has(edge.from) && shown.has(edge.to)).sort((a, b) => a.from.localeCompare(b.from) || a.to.localeCompare(b.to)),
  }
  return json(body)
}

/** Handles `/api/peerbonus/v1/admin/*`; undefined for anything else (the participant routes answer). */
export async function adminRoutes(request: Request, url: URL): Promise<Response | undefined> {
  if (!url.pathname.startsWith(`${BASE}/admin/`)) return undefined
  if (!isAdmin()) return forbidden()
  const route = `${request.method} ${url.pathname.slice(BASE.length)}`
  let match: RegExpMatchArray | null

  if (route === 'GET /admin/slack') return json(slackJson())
  if (route === 'PATCH /admin/slack') return updateSlack(request)
  if (route === 'POST /admin/slack/verify') return verifySlack(request)
  if (route === 'POST /admin/slack/test') return testSlack()
  if (route === 'GET /admin/slack/channels') return slackChannels(url)
  if (route === 'GET /admin/slack/deliveries') return slackDeliveries(url)
  if (route === 'GET /admin/program') return json(programAdminJson())
  if (route === 'PATCH /admin/program') return updateProgram(request)
  if (route === 'GET /admin/statements') return json({ items: statementSummaries() })
  if (route === 'GET /admin/network') {
    const month = url.searchParams.get('month') || monthKeyAt(mockNow(), mockState().program.timeZone)
    if (!isMonthKey(month)) return invalid('month must be YYYY-MM')
    return network(month, url.searchParams.get('group') ?? '')
  }
  if (route === 'GET /admin/members' || route === 'GET /admin/insights') {
    const month = url.searchParams.get('month') || monthKeyAt(mockNow(), mockState().program.timeZone)
    if (!isMonthKey(month)) return invalid('month must be YYYY-MM')
    return json(route === 'GET /admin/members' ? adminMembers(month) : insights(month))
  }
  if ((match = route.match(/^(GET|POST) \/admin\/statements\/([^/]+)(\/close|\/export\.csv)?$/))) {
    const [, method, rawMonth, action] = match
    const month = decodeURIComponent(rawMonth!)
    if (!isMonthKey(month)) return fail(400, 'invalid_month', 'month must be YYYY-MM')
    if (method === 'GET' && !action) return json(statementJson(month))
    if (method === 'POST' && action === '/close') return closeStatement(month)
    if (method === 'GET' && action === '/export.csv') {
      if (!mockState().statements[month]) return fail(409, 'statement_open', 'Only a closed month can be exported')
      return new Response(statementCsv(month), {
        headers: {
          'Content-Type': 'text/csv; charset=utf-8',
          'Content-Disposition': `attachment; filename="peerbonus-${month}.csv"`,
          'Cache-Control': 'no-store',
        },
      })
    }
  }
  return undefined
}

registerMockRoutes(adminRoutes)
registerMockRoutes(slackControl)
