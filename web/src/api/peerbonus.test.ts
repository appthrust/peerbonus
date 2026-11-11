import { beforeEach, describe, expect, test } from 'bun:test'
import '../dev/mock-admin'
import { MOCK_PERSON_IDS, peerbonusRoutes } from '../dev/mock-peerbonus'
import { QueryClient, type InfiniteData } from '@tanstack/react-query'
import { createPeerbonusApi, insufficientPoints, isPeerbonusError, PeerbonusApiError, slackTeamChange, statementCsvUrl, type Post, type PostPage } from './peerbonus'
import { prependPost, retryQuery, setInboxUnread, timelineQuery, meQuery } from './queries'

type Call = { input: string; init: RequestInit }

/** The client wired to the in-repo mock, recording every request it sends. */
function mockApi() {
  const calls: Call[] = []
  const api = createPeerbonusApi(async (input, init) => {
    calls.push({ input, init })
    const url = new URL(input, 'http://peerbonus.test')
    return (await peerbonusRoutes(new Request(url, init), url)) ?? new Response(null, { status: 404 })
  }, () => false)
  return { api, calls }
}

async function rejection(promise: Promise<unknown>): Promise<PeerbonusApiError> {
  try {
    await promise
  } catch (error) {
    if (error instanceof PeerbonusApiError) return error
    throw error
  }
  throw new Error('expected the call to fail')
}

const header = (call: Call | undefined, name: string) => new Headers(call?.init.headers).get(name)

beforeEach(async () => {
  const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/reset')
  await peerbonusRoutes(new Request(url, { method: 'POST' }), url)
})

describe('peerbonus API client', () => {
  test('reads go to same-origin /api/peerbonus/v1 without any token, with only the set parameters', async () => {
    const { api, calls } = mockApi()
    const me = await api.me()
    const page = await api.posts.list({ limit: 5, value: 'teamwork' })
    await api.posts.list({ cursor: page.nextCursor, sent: me.member.id })
    await api.members.search('佐藤', 5)
    await api.members.search('')
    await api.recipients()
    expect(calls.map((call) => call.input)).toEqual([
      '/api/peerbonus/v1/me',
      '/api/peerbonus/v1/posts?limit=5&value=teamwork',
      `/api/peerbonus/v1/posts?cursor=${encodeURIComponent(page.nextCursor!)}&sent=${me.member.id}`,
      `/api/peerbonus/v1/members?q=${encodeURIComponent('佐藤')}&limit=5`,
      '/api/peerbonus/v1/members',
      '/api/peerbonus/v1/recipients',
    ])
    for (const call of calls) {
      expect(call.init.credentials).toBe('same-origin')
      expect(new Headers(call.init.headers).has('Authorization')).toBe(false)
      expect(new Headers(call.init.headers).has('Content-Type')).toBe(false)
    }
    expect(statementCsvUrl('2026-10')).toBe('/api/peerbonus/v1/admin/statements/2026-10/export.csv')
  })

  test('a send carries JSON and its Idempotency-Key; resending the same key answers the same post', async () => {
    const { api, calls } = mockApi()
    const input = { recipientPersonId: MOCK_PERSON_IDS.sato, points: 10, message: 'ありがとう', values: ['teamwork'] }
    const post = await api.posts.create(input, 'key-1')
    expect(calls.at(-1)?.init).toMatchObject({ method: 'POST', body: JSON.stringify(input) })
    expect(header(calls.at(-1), 'Content-Type')).toBe('application/json')
    expect(header(calls.at(-1), 'Idempotency-Key')).toBe('key-1')
    const before = (await api.me()).stats.period.remaining
    expect((await api.posts.create(input, 'key-1')).id).toBe(post.id)
    expect((await api.me()).stats.period.remaining).toBe(before)
  })

  test('a clap is a body-less JSON POST with its key; a delete answers 204 without a body', async () => {
    const { api, calls } = mockApi()
    const mine = await api.posts.create({ recipientPersonId: MOCK_PERSON_IDS.sato, points: 10, message: 'x' }, 'send')
    const other = (await api.posts.list({ limit: 50 })).items.find((post) => post.sender.id !== mine.sender.id && post.recipient.id !== mine.sender.id)!
    const result = await api.posts.clap(other.id, 'clap-key')
    expect(result.post.clappedByMe).toBe(other.clappedByMe + 1)
    expect(calls.at(-1)?.init.body).toBeUndefined()
    expect(header(calls.at(-1), 'Content-Type')).toBe('application/json')
    expect(header(calls.at(-1), 'Idempotency-Key')).toBe('clap-key')
    expect((await api.posts.claps(other.id))[0]?.id).toBe(result.clap.id)
    expect(await api.posts.remove(other.id)).toBeUndefined()
    expect(calls.at(-1)?.init.method).toBe('DELETE')
    expect(header(calls.at(-1), 'Content-Type')).toBe('application/json')
  })

  test('errors keep the server code and body; unanswered calls are transient and retried, answers are final', async () => {
    const { api } = mockApi()
    const me = await api.me()
    const own = (await api.posts.list({ sent: me.member.id, limit: 1 })).items[0]!
    const ownPost = await rejection(api.posts.clap(own.id, 'k'))
    expect(ownPost).toMatchObject({ status: 403, code: 'own_post', transient: false })
    expect(retryQuery(0, ownPost)).toBe(false)
    expect(isPeerbonusError(await rejection(api.posts.get('missing')), 'not_found')).toBe(true)
    const overdraft = await rejection(api.posts.create({ recipientPersonId: MOCK_PERSON_IDS.sato, points: 120, message: 'x' }, 'a').then(() => api.posts.create({ recipientPersonId: MOCK_PERSON_IDS.sato, points: 120, message: 'x' }, 'b')).then(() => api.posts.create({ recipientPersonId: MOCK_PERSON_IDS.sato, points: 120, message: 'x' }, 'c')).then(() => api.posts.create({ recipientPersonId: MOCK_PERSON_IDS.sato, points: 120, message: 'x' }, 'd')))
    expect(overdraft.code).toBe('insufficient_points')
    expect(insufficientPoints(overdraft)).toEqual({ remaining: overdraft.body.remaining as number, required: 120 })
    expect(insufficientPoints(ownPost)).toBeUndefined()

    const offline = createPeerbonusApi(() => Promise.reject(new Error('offline')), () => false)
    const unanswered = await rejection(offline.me())
    expect(unanswered).toMatchObject({ status: 0, transient: true })
    expect(retryQuery(0, unanswered)).toBe(true)
    const proxy = createPeerbonusApi(async () => new Response('<html>bad gateway</html>', { status: 502 }), () => false)
    expect(await rejection(proxy.me())).toMatchObject({ status: 502, code: 'unknown', transient: true })
    const store = createPeerbonusApi(async () => Response.json({ error: 'peerbonus_store_unavailable', message: 'database unreachable' }, { status: 503 }), () => false)
    expect(await rejection(store.me())).toMatchObject({ status: 503, code: 'peerbonus_store_unavailable', transient: false })
  })

  test('admin routes: program, statements, members', async () => {
    const { api, calls } = mockApi()
    const program = await api.admin.program()
    expect(program).toMatchObject({ currency: 'JPY', pointUnitValue: '1' })
    expect((await api.admin.updateProgram({ weeklyAllowance: 500 })).weeklyAllowance).toBe(500)
    expect(calls.at(-1)?.init).toMatchObject({ method: 'PATCH', body: JSON.stringify({ weeklyAllowance: 500 }) })
    const statements = await api.admin.statements()
    expect(statements.length).toBeGreaterThanOrEqual(3)
    expect((await api.admin.statement(statements[0]!.month)).closed).toBe(false)
    expect((await api.admin.members()).items.length).toBeGreaterThan(0)
    expect(calls.at(-1)?.input).toBe('/api/peerbonus/v1/admin/members')
  })

  test('v2 routes: inbox paging and watermark, timeline text search, admin insights', async () => {
    const { api, calls } = mockApi()
    const page = await api.inbox.list({ limit: 2 })
    expect(page.items).toHaveLength(2)
    expect(page.items.some((item) => item.unread)).toBe(true)
    await api.inbox.list({ cursor: page.nextCursor })
    const seen = await api.inbox.markSeen()
    expect(seen.unread).toBe(0)
    expect((await api.me()).inbox.unread).toBe(0)
    await api.posts.list({ q: '100% 本気', month: '2026-10' })
    await api.admin.insights('2026-10')
    expect(calls.map((call) => `${call.init.method} ${call.input}`)).toEqual([
      'GET /api/peerbonus/v1/inbox?limit=2',
      `GET /api/peerbonus/v1/inbox?cursor=${encodeURIComponent(page.nextCursor!)}`,
      'POST /api/peerbonus/v1/inbox/seen',
      'GET /api/peerbonus/v1/me',
      `GET /api/peerbonus/v1/posts?month=2026-10&q=${encodeURIComponent('100% 本気').replace(/%20/g, '+')}`,
      'GET /api/peerbonus/v1/admin/insights?month=2026-10',
    ])
    expect(header(calls[2], 'Content-Type')).toBe('application/json')
  })

  test('Slack admin routes and /config; a refused verify carries both workspaces', async () => {
    const { api, calls } = mockApi()
    expect((await api.config()).projectId).toBe('pj-mock')
    expect((await api.admin.slack.get()).state).toBe('configured')
    expect((await api.admin.slack.verify()).state).toBe('connected')
    expect((await api.admin.slack.channels('gen')).map((channel) => channel.id)).toEqual(['C0GENERAL'])
    expect((await api.admin.slack.update({ announceChannelId: 'C0GENERAL', announcePosts: true })).settings.announceChannel).toEqual({ id: 'C0GENERAL', name: 'general' })
    expect((await api.admin.slack.test()).ok).toBe(true)
    expect(await api.admin.slack.deliveries('sent')).toHaveLength(1)
    expect(calls.map((call) => `${call.init.method} ${call.input}${call.init.body ? ` ${call.init.body}` : ''}`)).toEqual([
      'GET /config',
      'GET /api/peerbonus/v1/admin/slack',
      'POST /api/peerbonus/v1/admin/slack/verify {}',
      'GET /api/peerbonus/v1/admin/slack/channels?q=gen',
      'PATCH /api/peerbonus/v1/admin/slack {"announceChannelId":"C0GENERAL","announcePosts":true}',
      'POST /api/peerbonus/v1/admin/slack/test',
      'GET /api/peerbonus/v1/admin/slack/deliveries?state=sent',
    ])
    const other = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/slack')
    await peerbonusRoutes(new Request(other, { method: 'POST', body: JSON.stringify({ team: { teamId: 'T0OTHER', teamName: 'Other' } }) }), other)
    const refused = await rejection(api.admin.slack.verify())
    expect(slackTeamChange(refused)).toEqual({ current: { teamId: 'T0MOCKACME', teamName: 'Acme' }, token: { teamId: 'T0OTHER', teamName: 'Other' } })
    expect((await api.admin.slack.verify(true)).workspace?.teamName).toBe('Other')
    expect(calls.at(-1)?.init.body).toBe('{"force":true}')
  })
})

describe('query cache helpers', () => {
  const member = (id: string) => ({ id, personId: `p-${id}`, displayName: id, email: `${id}@example.com`, pending: false, directoryUrl: null, avatarUrl: null })
  const post = (id: string, extra: Partial<Post> = {}): Post => ({
    id,
    sender: member('me'),
    recipient: member('sato'),
    points: 30,
    message: 'ありがとう',
    values: ['teamwork'],
    clapCount: 0,
    clapPoints: 0,
    clappedByMe: 0,
    clappers: [],
    slackPermalink: null,
    period: '2026-W43',
    month: '2026-10',
    createdAt: '2026-10-19T00:00:00Z',
    ...extra,
  })
  const pages = (...items: Post[]): InfiniteData<PostPage> => ({ pages: [{ items, nextCursor: 'c' }, { items: [post('old')], nextCursor: null }], pageParams: [null, 'c'] })

  test('prependPost puts a sent post on top of every cached feed it belongs to, once', () => {
    const client = new QueryClient()
    const feeds = {
      all: timelineQuery({ kind: 'all' }),
      allBlank: timelineQuery({ kind: 'all', q: '  ', value: '' }),
      teamwork: timelineQuery({ kind: 'all', value: 'teamwork' }),
      other: timelineQuery({ kind: 'all', value: 'challenge' }),
      lastMonth: timelineQuery({ kind: 'all', month: '2026-09' }),
      text: timelineQuery({ kind: 'all', q: 'ありが' }),
      noText: timelineQuery({ kind: 'all', q: 'zzz' }),
      mine: timelineQuery({ kind: 'all', mine: true }),
      sent: timelineQuery({ kind: 'sent', memberId: 'me' }),
      received: timelineQuery({ kind: 'received', memberId: 'me' }),
    }
    // Unset filters share one cache entry.
    expect(feeds.allBlank.queryKey).toEqual(feeds.all.queryKey)
    for (const feed of Object.values(feeds)) client.setQueryData(feed.queryKey, pages(post('a')))
    const fresh = post('new')
    prependPost(client, fresh)
    const firstIds = (feed: (typeof feeds)[keyof typeof feeds]) => client.getQueryData(feed.queryKey)!.pages[0]!.items.map((item) => item.id)
    for (const name of ['all', 'teamwork', 'text', 'mine', 'sent'] as const) expect(firstIds(feeds[name])).toEqual(['new', 'a'])
    for (const name of ['other', 'lastMonth', 'noText', 'received'] as const) expect(firstIds(feeds[name])).toEqual(['a'])
    // Again (the refetch already had it): replaced in place, not doubled.
    prependPost(client, { ...fresh, clapCount: 1 })
    expect(firstIds(feeds.all)).toEqual(['new', 'a'])
    expect(client.getQueryData(feeds.all.queryKey)!.pages[0]!.items[0]!.clapCount).toBe(1)
    expect(client.getQueryData(feeds.all.queryKey)!.pages[1]!.items.map((item) => item.id)).toEqual(['old'])
  })

  test('setInboxUnread updates the cached me only when there is one', async () => {
    const client = new QueryClient()
    setInboxUnread(client, 0)
    expect(client.getQueryData(meQuery.queryKey)).toBeUndefined()
    const { api } = mockApi()
    client.setQueryData(meQuery.queryKey, await api.me())
    setInboxUnread(client, 0)
    expect(client.getQueryData(meQuery.queryKey)!.inbox.unread).toBe(0)
  })
})
