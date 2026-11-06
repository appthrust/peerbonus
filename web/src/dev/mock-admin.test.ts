import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import type { Insights, Participation, ProgramAdmin, SlackAdmin, SlackChannel, SlackDelivery, Statement, StatementSummary } from '../api/peerbonus'
import { CSV_HEADER } from './mock-admin'
import { MOCK_MEMBER_IDS, mockState, peerbonusRoutes, setMockClock } from './mock-peerbonus'

// 2026-10-20 12:00 in Asia/Tokyo; the seed reaches back about 70 days (August, September, October).
const NOW = Date.parse('2026-10-20T03:00:00Z')

async function call(method: string, path: string, body?: unknown) {
  const url = new URL(`/api/peerbonus/v1${path}`, 'http://mock.test')
  const response = await peerbonusRoutes(
    new Request(url, { method, body: body === undefined ? undefined : JSON.stringify(body), headers: { 'Content-Type': 'application/json' } }),
    url
  )
  if (!response) throw new Error(`unhandled ${method} ${path}`)
  const type = response.headers.get('Content-Type') ?? ''
  return {
    status: response.status,
    headers: response.headers,
    body: type.startsWith('text/csv') ? await response.text() : response.status === 204 ? undefined : await response.json(),
  }
}

const months = async () => ((await call('GET', '/admin/statements')).body.items as StatementSummary[]).map((item) => [item.month, item.closed])

beforeEach(async () => {
  setMockClock(NOW)
  expect((await call('POST', '/__mock/reset')).body).toEqual({ reset: true })
})

afterAll(() => setMockClock(null))

describe('admin mock: program', () => {
  test('GET carries the money settings and the time zone lock', async () => {
    const { status, body } = await call('GET', '/admin/program')
    expect(status).toBe(200)
    expect(body).toMatchObject({ timeZone: 'Asia/Tokyo', timeZoneLocked: true, weeklyAllowance: 400, maxPointsPerPost: 120, pointUnitValue: '1', currency: 'JPY', clapCost: 2 })
  })

  test('PATCH applies only the sent fields, replaces values and bumps updatedAt', async () => {
    const before: ProgramAdmin = (await call('GET', '/admin/program')).body
    const { status, body } = await call('PATCH', '/admin/program', {
      weeklyAllowance: 300,
      maxPointsPerPost: 300,
      pointUnitValue: '2.5',
      currency: 'USD',
      values: [{ key: 'kindness', label: '親切' }],
    })
    expect(status).toBe(200)
    expect(body).toMatchObject({ weeklyAllowance: 300, maxPointsPerPost: 300, pointUnitValue: '2.5', currency: 'USD', values: [{ key: 'kindness', label: '親切', description: null }] })
    expect(body.updatedAt > before.updatedAt).toBe(true)
    // A second save on the pinned clock is still a new version.
    const again: ProgramAdmin = (await call('PATCH', '/admin/program', { currency: 'JPY' })).body
    expect(again.updatedAt > body.updatedAt).toBe(true)
    expect(again.weeklyAllowance).toBe(300)
  })

  test('PATCH rejects what domain.ValidateProgramPatch rejects', async () => {
    const rejected = [
      { weeklyAllowance: 10_001 },
      { weeklyAllowance: 100 }, // below the stored maxPointsPerPost 120
      { maxPointsPerPost: 0 },
      { maxPointsPerPost: 401 },
      { weeklyAllowance: 1.5 },
      { pointUnitValue: '-1' },
      { pointUnitValue: '1.' },
      { currency: 'jpy' },
      { timeZone: 'Mars/Olympus' },
      { values: [{ key: 'Bad Key', label: 'x' }] },
      { values: [{ key: 'a', label: ' ' }] },
      { values: [{ key: 'a', label: 'A' }, { key: 'a', label: 'B' }] },
      { values: Array.from({ length: 21 }, (_, i) => ({ key: `v${i}`, label: `V${i}` })) },
    ]
    for (const patch of rejected) {
      const { status, body } = await call('PATCH', '/admin/program', patch)
      expect([status, body.error, JSON.stringify(patch)]).toEqual([400, 'invalid_request', JSON.stringify(patch)])
    }
  })

  test('the time zone locks once the ledger has a row', async () => {
    expect((await call('PATCH', '/admin/program', { timeZone: 'Asia/Tokyo' })).status).toBe(200)
    const locked = await call('PATCH', '/admin/program', { timeZone: 'UTC', weeklyAllowance: 200 })
    expect([locked.status, locked.body.error]).toEqual([409, 'timezone_locked'])
    expect((await call('GET', '/admin/program')).body.weeklyAllowance).toBe(400)

    mockState().ledger.length = 0
    const free = await call('PATCH', '/admin/program', { timeZone: 'UTC' })
    expect(free.status).toBe(200)
    expect(free.body).toMatchObject({ timeZone: 'UTC', timeZoneLocked: false })
  })
})

describe('admin mock: statements', () => {
  test('the list runs from the oldest ledger month to the current month, newest first', async () => {
    expect(await months()).toEqual([
      ['2026-10', false],
      ['2026-09', false],
      ['2026-08', false],
    ])
    const [current] = (await call('GET', '/admin/statements')).body.items as StatementSummary[]
    const preview: Statement = (await call('GET', '/admin/statements/2026-10')).body
    expect(preview).toMatchObject({ month: '2026-10', closed: false, closedAt: null, closedBy: null, directoryRefreshed: false, pointUnitValue: '1', currency: 'JPY' })
    expect(current).toMatchObject({ totalPoints: preview.totalPoints, totalAmount: preview.totalAmount, memberCount: preview.lines.length })
    expect(preview.totalPoints).toBeGreaterThan(0)
    expect(preview.lines.every((line) => line.points > 0)).toBe(true)
    expect(preview.lines.every((line) => Object.hasOwn(line.member, 'avatarUrl'))).toBe(true)
    expect(preview.lines.map((line) => line.points)).toEqual([...preview.lines.map((line) => line.points)].sort((a, b) => b - a))
  })

  test('close blockers describe ended months, earlier ledger months, and closed months', async () => {
    expect((await call('GET', '/admin/statements/2026-08')).body).toMatchObject({ closeBlockers: [], previousOpenMonth: null })
    expect((await call('GET', '/admin/statements/2026-09')).body).toMatchObject({ closeBlockers: ['previous_month_open'], previousOpenMonth: '2026-08' })
    expect((await call('GET', '/admin/statements/2026-10')).body).toMatchObject({ closeBlockers: ['month_not_ended', 'previous_month_open'], previousOpenMonth: '2026-08' })
    await call('POST', '/admin/statements/2026-08/close')
    expect((await call('GET', '/admin/statements/2026-08')).body).toMatchObject({ closeBlockers: [], previousOpenMonth: null })
    expect((await call('GET', '/admin/statements/2026-09')).body).toMatchObject({ closeBlockers: [], previousOpenMonth: null })
    expect((await call('GET', '/admin/statements/2026-10')).body.previousOpenMonth).toBe('2026-09')
    await call('POST', '/admin/statements/2026-09/close')
    expect((await call('GET', '/admin/statements/2026-10')).body).toMatchObject({ closeBlockers: ['month_not_ended'], previousOpenMonth: null })
  })

  test('close: only after the month ended, oldest first, idempotent', async () => {
    const early = await call('POST', '/admin/statements/2026-10/close')
    expect([early.status, early.body.error]).toEqual([409, 'month_not_ended'])
    const skipped = await call('POST', '/admin/statements/2026-09/close')
    expect([skipped.status, skipped.body.error]).toEqual([409, 'previous_month_open'])

    const august = await call('POST', '/admin/statements/2026-08/close')
    expect(august.status).toBe(200)
    const closed: Statement = august.body
    expect(closed).toMatchObject({ month: '2026-08', closed: true, directoryRefreshed: true, pointUnitValue: '1', currency: 'JPY' })
    expect(closed.closedBy?.id).toBe(MOCK_MEMBER_IDS.yamada)
    expect(closed.closedBy?.avatarUrl).toBe(`/api/peerbonus/v1/avatars/p-yamada?v=${encodeURIComponent('2026-10-01T09:00:00.000Z')}`)
    expect(closed.closedAt).toBe(new Date(NOW).toISOString())

    setMockClock(NOW + 60_000)
    const replay = await call('POST', '/admin/statements/2026-08/close')
    expect(replay.body).toEqual(closed)

    expect((await call('POST', '/admin/statements/2026-09/close')).status).toBe(200)
    expect(await months()).toEqual([
      ['2026-10', false],
      ['2026-09', true],
      ['2026-08', true],
    ])
    expect((await call('POST', '/admin/statements/2026-13/close')).body.error).toBe('invalid_month')
  })

  test('the month ends at midnight in the program time zone (injectable clock)', async () => {
    setMockClock(Date.parse('2026-11-05T00:00:00Z'))
    await call('POST', '/admin/statements/2026-08/close')
    await call('POST', '/admin/statements/2026-09/close')
    // 2026-10-31 23:59:59 in Tokyo.
    setMockClock(Date.parse('2026-10-31T14:59:59Z'))
    expect((await call('POST', '/admin/statements/2026-10/close')).body.error).toBe('month_not_ended')
    // 2026-11-01 00:00 in Tokyo.
    setMockClock(Date.parse('2026-10-31T15:00:00Z'))
    expect((await call('POST', '/admin/statements/2026-10/close')).status).toBe(200)
  })

  test('closing freezes the rate and currency; open months follow the current settings', async () => {
    const closed: Statement = (await call('POST', '/admin/statements/2026-08/close')).body
    await call('PATCH', '/admin/program', { pointUnitValue: '2.5', currency: 'USD' })

    const frozen: Statement = (await call('GET', '/admin/statements/2026-08')).body
    expect(frozen).toEqual(closed)
    expect(frozen.lines[0]!.amount).toBe(`${frozen.lines[0]!.points}.00`)

    const open: Statement = (await call('GET', '/admin/statements/2026-09')).body
    expect(open).toMatchObject({ pointUnitValue: '2.5', currency: 'USD' })
    for (const line of open.lines) expect(line.amount).toBe((line.points * 2.5).toFixed(2))
    expect(open.totalAmount).toBe((open.totalPoints * 2.5).toFixed(2))

    await call('PATCH', '/admin/program', { pointUnitValue: '0.125' })
    const fine: Statement = (await call('GET', '/admin/statements/2026-09')).body
    expect(fine.lines[0]!.amount).toBe((fine.lines[0]!.points * 0.125).toFixed(3))
  })

  test('directoryRefreshed is false when Directory is down at close', async () => {
    await call('POST', '/__mock/directory', { up: false })
    const closed: Statement = (await call('POST', '/admin/statements/2026-08/close')).body
    expect(closed.directoryRefreshed).toBe(false)
  })

  test('CSV: closed months only, BOM, the spec header and one row per line', async () => {
    const open = await call('GET', '/admin/statements/2026-08/export.csv')
    expect([open.status, open.body.error]).toEqual([409, 'statement_open'])

    const closed: Statement = (await call('POST', '/admin/statements/2026-08/close')).body
    const csv = await call('GET', '/admin/statements/2026-08/export.csv')
    expect(csv.status).toBe(200)
    expect(csv.headers.get('Content-Type')).toBe('text/csv; charset=utf-8')
    expect(csv.headers.get('Content-Disposition')).toBe('attachment; filename="peerbonus-2026-08.csv"')
    const text: string = csv.body
    expect(text.charCodeAt(0)).toBe(0xfeff)
    const rows = text.slice(1).trimEnd().split('\n')
    expect(rows[0]).toBe(CSV_HEADER)
    expect(rows).toHaveLength(closed.lines.length + 1)
    const first = closed.lines[0]!
    expect(rows[1]).toBe(
      ['2026-08', first.member.personId, first.member.email, first.member.displayName, String(first.member.pending), first.points, '1', 'JPY', first.amount].join(',')
    )
  })

  test('CSV quotes fields with commas or quotes', async () => {
    const closed: Statement = (await call('POST', '/admin/statements/2026-08/close')).body
    mockState().members.find((candidate) => candidate.id === closed.lines[0]!.member.id)!.displayName = 'Doe, "J"'
    const text: string = (await call('GET', '/admin/statements/2026-08/export.csv')).body
    expect(text).toContain(',"Doe, ""J""",')
  })
})

describe('admin mock: members and access', () => {
  test('members of a month, most points received first', async () => {
    const { status, body } = await call('GET', '/admin/members?month=2026-09')
    expect(status).toBe(200)
    expect(body.month).toBe('2026-09')
    const received = (body.items as { receivedPoints: number }[]).map((row) => row.receivedPoints)
    expect(received.length).toBeGreaterThan(0)
    expect(received).toEqual([...received].sort((a, b) => b - a))
    const statement: Statement = (await call('GET', '/admin/statements/2026-09')).body
    expect(received.reduce((sum, value) => sum + value, 0)).toBe(statement.totalPoints)

    expect((await call('GET', '/admin/members')).body.month).toBe('2026-10')
    expect((await call('GET', '/admin/members?month=2026-9')).status).toBe(400)
  })

  test('participation includes every Directory person, with null member for people who never received', async () => {
    const body: Participation = (await call('GET', '/admin/members')).body
    expect(body.directoryAvailable).toBe(true)
    expect(body.summary.people).toBe(9)
    expect(body.items).toHaveLength(9)
    expect(body.items.find((row) => row.personId === 'p-kobayashi')).toMatchObject({
      displayName: '小林 優', pending: true, member: null, sentPosts: 0, sentPoints: 0, receivedPosts: 0, receivedPoints: 0, clapsGiven: 0,
    })
    expect(body.items.find((row) => row.personId === 'p-kobayashi')?.avatarUrl).toBe(`/api/peerbonus/v1/avatars/p-kobayashi?v=${encodeURIComponent('2026-10-01T09:00:00.000Z')}`)
    expect(body.items.find((row) => row.personId === 'p-suzuki')?.avatarUrl).toBeNull()
    expect(body.items.every((row) => row.member === null || row.member.avatarUrl === row.avatarUrl)).toBe(true)
    expect(body.summary.senders).toBe(body.items.filter((row) => row.sentPosts > 0).length)
    expect(body.summary.recipients).toBe(body.items.filter((row) => row.receivedPosts > 0).length)
    expect(body.summary.active).toBe(body.items.filter((row) => row.sentPosts + row.receivedPosts + row.clapsGiven > 0 || row.receivedPoints > 0).length)
    await call('POST', '/__mock/directory', { up: false })
    const degraded: Participation = (await call('GET', '/admin/members')).body
    expect(degraded.directoryAvailable).toBe(false)
    expect(degraded.summary.people).toBe(8)
    expect(degraded.items.every((row) => row.member !== null)).toBe(true)
  })

  test('insights totals match the ledger and weeks cover the month including zero activity', async () => {
    const body: Insights = (await call('GET', '/admin/insights?month=2026-10')).body
    const statement: Statement = (await call('GET', '/admin/statements/2026-10')).body
    expect(body.points).toBe(statement.totalPoints)
    expect(body.posts).toBe(mockState().posts.filter((post) => post.monthKey === body.month && !post.deletedAt).length)
    expect(body.weeks.map((week) => week.key)).toEqual(['2026-W40', '2026-W41', '2026-W42', '2026-W43', '2026-W44'])
    expect(body.weeks[0]!.startsAt).toBe('2026-09-27T15:00:00.000Z')
    expect(body.weeks.at(-1)!.endsAt).toBe('2026-11-01T15:00:00.000Z')
    expect(body.weeks.at(-1)).toMatchObject({ posts: 0, points: 0, claps: 0 })
    expect(body.weeks.reduce((sum, week) => sum + week.points, 0)).toBe(body.points)
    expect(body.weeks.reduce((sum, week) => sum + week.posts, 0)).toBe(body.posts)
    expect(body.weeks.reduce((sum, week) => sum + week.claps, 0)).toBe(body.claps)
    expect(body.values.map((value) => value.posts)).toEqual(body.values.map((value) => value.posts).sort((a, b) => b - a))
    expect(body.topRecipients.length).toBeLessThanOrEqual(5)
    expect(body.topSenders.length).toBeLessThanOrEqual(5)
    const empty: Insights = (await call('GET', '/admin/insights?month=2027-01')).body
    expect(empty).toMatchObject({ points: 0, posts: 0, claps: 0, senders: 0, recipients: 0, values: [], topRecipients: [], topSenders: [] })
    expect(empty.weeks).toHaveLength(5)
    expect(empty.weeks.every((week) => week.posts === 0 && week.points === 0 && week.claps === 0)).toBe(true)
    expect((await call('GET', '/admin/insights?month=2026-13')).status).toBe(400)
  })

  test('every admin route is 403 for a participant', async () => {
    await call('POST', '/__mock/caller', { memberId: MOCK_MEMBER_IDS.sato })
    for (const [method, path] of [
      ['GET', '/admin/program'],
      ['PATCH', '/admin/program'],
      ['GET', '/admin/statements'],
      ['GET', '/admin/statements/2026-08'],
      ['POST', '/admin/statements/2026-08/close'],
      ['GET', '/admin/statements/2026-08/export.csv'],
      ['GET', '/admin/members'],
      ['GET', '/admin/insights'],
      ['GET', '/admin/slack'],
      ['PATCH', '/admin/slack'],
      ['POST', '/admin/slack/verify'],
      ['GET', '/admin/slack/channels'],
      ['POST', '/admin/slack/test'],
      ['GET', '/admin/slack/deliveries'],
    ] as const) {
      const { status, body } = await call(method, path, method === 'PATCH' ? {} : undefined)
      expect([path, status, body.error]).toEqual([path, 403, 'forbidden'])
    }
  })
})

describe('admin mock: Slack', () => {
  const slack = async (): Promise<SlackAdmin> => (await call('GET', '/admin/slack')).body
  const connect = async () => expect((await call('POST', '/admin/slack/verify', {})).status).toBe(200)

  test('starts configured: token set, nothing verified, default settings, no inbound', async () => {
    expect(await slack()).toEqual({
      credential: { botToken: true, signingSecret: false },
      state: 'configured',
      workspace: null,
      settings: { announceChannel: null, announcePosts: false, dmRecipientOnPost: true, giveFromSlack: false, locale: 'ja' },
      inbound: null,
      lastError: null,
      deliveries: { pending: 0, failed24h: 0, sent24h: 0 },
    })
  })

  test('verify fixes the workspace; settings are 409 until then; no token is 409 slack_not_configured', async () => {
    expect((await call('PATCH', '/admin/slack', { locale: 'en' })).body.error).toBe('slack_not_connected')
    const { status, body } = await call('POST', '/admin/slack/verify', {})
    expect(status).toBe(200)
    expect(body).toMatchObject({ state: 'connected', workspace: { teamId: 'T0MOCKACME', teamName: 'Acme', botUserId: 'U0PEERBONUS', verifiedAt: new Date(NOW).toISOString() } })
    await call('POST', '/__mock/slack', { botToken: false })
    expect((await slack()).state).toBe('unconfigured')
    expect((await call('POST', '/admin/slack/verify', {})).body.error).toBe('slack_not_configured')
  })

  test('PATCH persists a channel and flags, and refuses what the server refuses', async () => {
    await connect()
    const { status, body } = await call('PATCH', '/admin/slack', { announceChannelId: 'C0GENERAL', announcePosts: true, locale: 'en' })
    expect(status).toBe(200)
    expect(body.settings).toEqual({ announceChannel: { id: 'C0GENERAL', name: 'general' }, announcePosts: true, dmRecipientOnPost: true, giveFromSlack: false, locale: 'en' })
    expect((await slack()).settings).toEqual(body.settings)
    expect((await call('PATCH', '/admin/slack', { announceChannelId: 'C0NOPE' })).status).toBe(400)
    expect((await call('PATCH', '/admin/slack', { announceChannelId: null })).status).toBe(400)
    expect((await call('PATCH', '/admin/slack', { announceChannelId: null, announcePosts: false })).body.settings.announceChannel).toBeNull()
    expect((await call('PATCH', '/admin/slack', { locale: 'fr' })).status).toBe(400)
    expect((await call('PATCH', '/admin/slack', { giveFromSlack: true })).body.error).toBe('slack_signing_secret_missing')
    await call('POST', '/__mock/slack', { signingSecret: true })
    expect((await call('PATCH', '/admin/slack', { giveFromSlack: true })).body.settings.giveFromSlack).toBe(true)
  })

  test('channels: prefix search with the bot membership, at most the limit', async () => {
    const names = async (query: string) => ((await call('GET', `/admin/slack/channels${query}`)).body.items as SlackChannel[]).map((channel) => `${channel.name}:${channel.isMember}`)
    expect(await names('?q=sal')).toEqual(['sales:false', 'sales-jp:false'])
    expect(await names('?q=%23gen')).toEqual(['general:true'])
    expect(await names('?limit=2')).toEqual(['general:true', 'kudos:true'])
    expect((await call('GET', '/admin/slack/channels?limit=21')).status).toBe(400)
  })

  test('a test message to a channel without the bot is 409; after /invite it records a sent delivery', async () => {
    expect((await call('POST', '/admin/slack/test')).body.error).toBe('slack_not_connected')
    await connect()
    await call('PATCH', '/admin/slack', { announceChannelId: 'C0RANDOM', announcePosts: true })
    expect((await call('POST', '/admin/slack/test')).body.error).toBe('slack_bot_not_in_channel')
    await call('POST', '/__mock/slack', { invite: 'C0RANDOM' })
    const sent = await call('POST', '/admin/slack/test')
    expect(sent.status).toBe(200)
    expect(sent.body).toMatchObject({ ok: true })
    const items: SlackDelivery[] = (await call('GET', '/admin/slack/deliveries')).body.items
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ kind: 'post_announce', state: 'sent', attempts: 1, lastError: null, sentAt: new Date(NOW).toISOString() })
    expect((await slack()).deliveries).toEqual({ pending: 0, failed24h: 0, sent24h: 1 })
    expect((await call('GET', '/admin/slack/deliveries?state=failed')).body.items).toEqual([])
    expect((await call('GET', '/admin/slack/deliveries?state=nope')).status).toBe(400)
  })

  test('a token of another workspace pauses (team_changed) until a forced verify, which drops the channel', async () => {
    await connect()
    await call('PATCH', '/admin/slack', { announceChannelId: 'C0GENERAL', announcePosts: true })
    await call('POST', '/__mock/slack', { team: { teamId: 'T0OTHER', teamName: 'Other' } })
    expect(await slack()).toMatchObject({ state: 'team_changed', lastError: { code: 'team_changed' } })
    const refused = await call('POST', '/admin/slack/verify', {})
    expect(refused.status).toBe(409)
    expect(refused.body).toMatchObject({ error: 'slack_team_changed', detail: { current: { teamId: 'T0MOCKACME', teamName: 'Acme' }, token: { teamId: 'T0OTHER', teamName: 'Other' } } })
    const forced = await call('POST', '/admin/slack/verify', { force: true })
    expect(forced.body).toMatchObject({ state: 'connected', workspace: { teamId: 'T0OTHER', teamName: 'Other' }, lastError: null })
    expect(forced.body.settings).toMatchObject({ announceChannel: null, announcePosts: false })
  })

  test('the inbound endpoint shows once the environment has one; reset starts Slack over', async () => {
    const inbound = { url: 'https://in.example/pj-mock/peerbonus-slack', ready: true, reason: null }
    await call('POST', '/__mock/slack', { inbound })
    expect((await slack()).inbound).toEqual(inbound)
    await connect()
    await call('POST', '/__mock/reset')
    expect(await slack()).toMatchObject({ state: 'configured', inbound: null })
  })
})
