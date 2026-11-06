import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import type { InboxPage, Me, MemberProfile, Post, PostPage, Recipients } from '../api/peerbonus'
import { MOCK_GROUPS, MOCK_MEMBER_IDS, MOCK_PERSON_IDS, mockState, peerbonusRoutes, setMockClock, type MockStatement } from './mock-peerbonus'

// Wednesday 2026-10-21 12:00 in Tokyo: week 2026-W43 (Mon 10-19 .. Mon 10-26), month 2026-10.
const NOW = Date.parse('2026-10-21T03:00:00Z')

let keys = 0
async function call(method: string, path: string, options: { body?: unknown; key?: string | null } = {}) {
  const url = new URL(path.startsWith('/config') ? path : `/api/peerbonus/v1${path}`, 'http://mock.test')
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (method === 'POST' && options.key !== null && !path.startsWith('/__mock')) headers['Idempotency-Key'] = options.key ?? `key-${keys++}`
  const response = await peerbonusRoutes(new Request(url, { method, headers, body: options.body === undefined ? undefined : JSON.stringify(options.body) }), url)
  if (!response) throw new Error(`unhandled ${method} ${path}`)
  return { status: response.status, body: response.status === 204 ? undefined : await response.json() }
}

const as = (member: keyof typeof MOCK_MEMBER_IDS, admin = false) => call('POST', '/__mock/caller', { body: { memberId: MOCK_MEMBER_IDS[member], admin } })
const me = async () => (await call('GET', '/me')).body as Me
const send = (recipient: keyof typeof MOCK_PERSON_IDS, points: number, extra: Record<string, unknown> = {}, key?: string) =>
  call('POST', '/posts', { body: { recipientPersonId: MOCK_PERSON_IDS[recipient], points, message: 'ありがとう！', ...extra }, key })
const remainingOf = async (member: keyof typeof MOCK_MEMBER_IDS) => {
  await as(member)
  return (await me()).stats.period.remaining
}

beforeEach(async () => {
  setMockClock(NOW)
  expect(await call('POST', '/__mock/reset')).toEqual({ status: 200, body: { reset: true } })
})
afterAll(() => setMockClock(null))

describe('peerbonus mock contract', () => {
  test('avatars use versioned same-origin URLs on me, posts and candidates, or null without a picture', async () => {
    const version = '2026-10-01T09:00:00.000Z'
    const avatarUrl = `/api/peerbonus/v1/avatars/p-yamada?v=${encodeURIComponent(version)}`
    expect((await me()).member.avatarUrl).toBe(avatarUrl)
    const post = (await send('sato', 10)).body as Post
    expect(post.sender.avatarUrl).toBe(avatarUrl)
    expect(post.recipient.avatarUrl).toBe(`/api/peerbonus/v1/avatars/p-sato?v=${encodeURIComponent(version)}`)
    const recipients: Recipients = (await call('GET', '/recipients')).body
    expect(recipients.everyone.items.find((person) => person.personId === MOCK_PERSON_IDS.sato)?.avatarUrl).toBe(post.recipient.avatarUrl)
    expect(recipients.everyone.items.find((person) => person.personId === MOCK_PERSON_IDS.suzuki)?.avatarUrl).toBeNull()
    await as('suzuki')
    expect((await me()).member.avatarUrl).toBeNull()
    expect(mockState().people.filter((person) => person.avatarVersion !== null)).toHaveLength(7)
    expect(mockState().people.filter((person) => person.avatarVersion === null)).toHaveLength(2)
  })

  test('avatar route serves PNG bytes with private caching and ignores the version query', async () => {
    const request = async (path: string) => {
      const url = new URL(`/api/peerbonus/v1${path}`, 'http://mock.test')
      return (await peerbonusRoutes(new Request(url), url))!
    }
    const response = await request('/avatars/p-yamada?v=stale')
    expect(response.status).toBe(200)
    expect(response.headers.get('Content-Type')).toBe('image/png')
    expect(response.headers.get('Cache-Control')).toBe('private, max-age=86400')
    const png = new Uint8Array(await response.arrayBuffer())
    expect([...png.subarray(0, 8)]).toEqual([137, 80, 78, 71, 13, 10, 26, 10])
    const sato = await request('/avatars/p-sato')
    expect(new Uint8Array(await sato.arrayBuffer())).not.toEqual(png)
    for (const id of ['p-suzuki', 'p-watanabe', 'p-unknown']) {
      const missing = await request(`/avatars/${id}`)
      expect(missing.status).toBe(404)
      expect(await missing.json()).toMatchObject({ error: 'not_found' })
    }
  })

  test('GET /me carries the member, admin role, Program, this week and stats; /config the origins', async () => {
    const body = await me()
    expect(body.member).toMatchObject({ id: MOCK_MEMBER_IDS.yamada, displayName: '山田 太郎', pending: false, directoryUrl: 'https://directory.pj-mock.appthrust.ink/people/p-yamada' })
    expect(body.roles.peerbonusAdmin).toBe(true)
    expect(body.program).toMatchObject({ timeZone: 'Asia/Tokyo', timeZoneLocked: true, weeklyAllowance: 400, maxPointsPerPost: 120, clapCost: 2 })
    expect(body.program).not.toHaveProperty('pointUnitValue')
    expect(body.period).toMatchObject({ key: '2026-W43', startsAt: '2026-10-18T15:00:00.000Z', endsAt: '2026-10-25T15:00:00.000Z', allowance: 400, maxPointsPerPost: 120 })
    expect(body.stats.period.remaining).toBeGreaterThan(0)
    expect(body.stats.period.remaining).toBeLessThanOrEqual(400)
    expect(body.stats.month.key).toBe('2026-10')
    expect((await call('GET', '/config')).body).toEqual({ projectId: 'pj-mock', publicUrl: 'https://peerbonus.pj-mock.appthrust.ink', directoryUrl: 'https://directory.pj-mock.appthrust.ink' })
    await as('suzuki')
    expect((await me()).member.pending).toBe(true)
    expect((await me()).roles.peerbonusAdmin).toBe(false)
  })

  test('a send debits the sender, credits the recipient and leads the timeline; the same key replays the post', async () => {
    const before = await me()
    const satoBefore = (await call('GET', `/members/${MOCK_MEMBER_IDS.sato}`)).body.stats.month.receivedPoints
    const sent = await send('sato', 30, { values: ['challenge', 'thanks'] }, 'once')
    expect(sent.status).toBe(201)
    const post = sent.body as Post
    expect(post).toMatchObject({ points: 30, values: ['challenge', 'thanks'], clapCount: 0, clappedByMe: 0, period: '2026-W43', month: '2026-10' })
    expect(post.sender.id).toBe(MOCK_MEMBER_IDS.yamada)
    expect(post.recipient.personId).toBe('p-sato')
    expect((await me()).stats.period.remaining).toBe(before.stats.period.remaining - 30)
    expect((await call('GET', `/members/${MOCK_MEMBER_IDS.sato}`)).body.stats.month.receivedPoints).toBe(satoBefore + 30)
    expect(((await call('GET', '/posts?limit=1')).body as PostPage).items[0]?.id).toBe(post.id)

    const replay = await send('sato', 30, { values: ['challenge', 'thanks'] }, 'once')
    expect(replay).toEqual({ status: 201, body: post })
    expect((await me()).stats.period.remaining).toBe(before.stats.period.remaining - 30)
  })

  test('sending to a Directory-only person creates a pending member', async () => {
    const post = (await send('kobayashi', 10)).body as Post
    expect(post.recipient).toMatchObject({ personId: 'p-kobayashi', displayName: '小林 優', pending: true })
    expect((await call('GET', `/members/${post.recipient.id}`)).status).toBe(200)
  })

  test('validation: key, points range, message, values, self and unknown recipients', async () => {
    expect((await call('POST', '/posts', { body: { recipientPersonId: 'p-sato', points: 1, message: 'x' }, key: null })).body.error).toBe('invalid_request')
    expect((await send('sato', 0)).body.error).toBe('invalid_request')
    expect((await send('sato', 121)).body.error).toBe('invalid_request')
    expect((await send('sato', 1.5)).body.error).toBe('invalid_request')
    expect((await send('sato', 10, { message: '   ' })).body.error).toBe('invalid_request')
    expect((await send('sato', 10, { message: 'あ'.repeat(501) })).body.error).toBe('invalid_request')
    expect((await send('sato', 10, { message: 'あ'.repeat(500) })).status).toBe(201)
    expect((await send('sato', 10, { values: ['a', 'b', 'c', 'd'] })).body.error).toBe('invalid_request')
    expect((await send('sato', 10, { values: ['thanks', 'thanks'] })).body.error).toBe('invalid_request')
    expect(await send('sato', 10, { values: ['nope'] })).toMatchObject({ status: 400, body: { error: 'unknown_value' } })
    expect(await send('yamada', 10)).toMatchObject({ status: 400, body: { error: 'self_target' } })
    expect(await call('POST', '/posts', { body: { recipientPersonId: 'p-ghost', points: 1, message: 'x' } })).toMatchObject({ status: 404, body: { error: 'recipient_not_found' } })
  })

  test('the weekly balance refuses an overdraft with remaining and required', async () => {
    let left = (await me()).stats.period.remaining
    while (left >= 120) {
      expect((await send('tanaka', 120)).status).toBe(201)
      left -= 120
    }
    const refused = await send('tanaka', left + 1)
    expect(refused).toMatchObject({ status: 409, body: { error: 'insufficient_points', remaining: left, required: left + 1 } })
  })

  test('a clap costs the clapper 2 and credits sender and recipient 1 each; never on one’s own post', async () => {
    await as('sato')
    const post = (await send('tanaka', 20)).body as Post
    expect(await call('POST', `/posts/${post.id}/claps`)).toMatchObject({ status: 403, body: { error: 'own_post' } })
    const kato = await remainingOf('kato')
    const received = async (member: keyof typeof MOCK_MEMBER_IDS) => (await call('GET', `/members/${MOCK_MEMBER_IDS[member]}`)).body.stats.month.receivedPoints as number
    const [sato, tanaka] = [await received('sato'), await received('tanaka')]
    const clapped = await call('POST', `/posts/${post.id}/claps`, { key: 'clap-1' })
    expect(clapped.status).toBe(201)
    expect(clapped.body.clap.member.id).toBe(MOCK_MEMBER_IDS.kato)
    expect(clapped.body.post).toMatchObject({ clapCount: 1, clapPoints: 2, clappedByMe: 1 })
    expect((await call('POST', `/posts/${post.id}/claps`, { key: 'clap-1' })).body.clap.id).toBe(clapped.body.clap.id)
    expect((await call('POST', `/posts/${post.id}/claps`)).body.post.clappedByMe).toBe(2)
    expect((await me()).stats.period.remaining).toBe(kato - 4)
    expect([await received('sato'), await received('tanaka')]).toEqual([sato + 2, tanaka + 2])
    expect((await call('GET', `/posts/${post.id}/claps`)).body.items).toHaveLength(2)
  })

  test('a closed month refuses claps on its posts', async () => {
    const post = (await send('tanaka', 20)).body as Post
    const statement: MockStatement = { month: '2026-10', closedAt: new Date(NOW).toISOString(), closedBy: MOCK_MEMBER_IDS.yamada, pointUnitValue: '1', currency: 'JPY', directoryRefreshed: true, lines: [] }
    mockState().statements['2026-10'] = statement
    await as('kato')
    expect(await call('POST', `/posts/${post.id}/claps`)).toMatchObject({ status: 409, body: { error: 'period_closed' } })
  })

  test('only an admin deletes; an open month reverses every point the post moved', async () => {
    await as('sato')
    const post = (await send('tanaka', 50)).body as Post
    await as('kato')
    await call('POST', `/posts/${post.id}/claps`)
    expect((await call('DELETE', `/posts/${post.id}`)).body.error).toBe('forbidden')
    expect(await remainingOf('sato')).toBeLessThan(400)
    const satoBefore = await remainingOf('sato')
    await as('yamada', true)
    expect((await call('DELETE', `/posts/${post.id}`)).status).toBe(204)
    expect((await call('GET', `/posts/${post.id}`)).status).toBe(404)
    expect(await remainingOf('sato')).toBe(satoBefore + 50)
    expect(mockState().posts.find((candidate) => candidate.id === post.id)?.reversed).toBe(true)
    expect(mockState().ledger.filter((row) => row.postId === post.id && row.kind === 'reversal')).toHaveLength(5)
  })

  test('search reads Directory without the caller; Directory down only blocks unknown recipients', async () => {
    const found = (await call('GET', '/members?q=example.com&limit=20')).body.items as { personId: string }[]
    expect(found.map((item) => item.personId)).not.toContain('p-yamada')
    expect(found).toHaveLength(8)
    const everyone = (await call('GET', '/members?limit=20')).body.items as { displayName: string; groups: unknown[] }[]
    expect(everyone).toHaveLength(8)
    expect(everyone.map((item) => item.displayName)).toEqual(everyone.map((item) => item.displayName).sort((a, b) => a.localeCompare(b, 'ja')))
    expect(everyone.every((item) => Array.isArray(item.groups) && item.groups.length === 0)).toBe(true)
    expect((await call('GET', `/members?q=${'x'.repeat(65)}`)).status).toBe(400)
    expect((await call('GET', `/members?q=${encodeURIComponent('佐藤')}`)).body.items[0].displayName).toBe('佐藤 花子')
    await call('POST', '/__mock/directory', { body: { up: false } })
    expect(await call('GET', '/members?q=sato')).toMatchObject({ status: 503, body: { error: 'directory_unavailable' } })
    expect((await send('sato', 5)).status).toBe(201)
    expect(await send('kobayashi', 5)).toMatchObject({ status: 503, body: { error: 'directory_unavailable' } })
  })

  test('recipients: recent sends, the caller’s direct teams deepest first, everyone; Directory down keeps recent', async () => {
    await send('kobayashi', 5)
    const body: Recipients = (await call('GET', '/recipients')).body
    expect(body.directoryAvailable).toBe(true)
    expect(body.me).toEqual({ personId: MOCK_PERSON_IDS.yamada, groups: [MOCK_GROUPS.teamA, MOCK_GROUPS.ceatec] })
    expect(body.recent[0]).toMatchObject({ personId: MOCK_PERSON_IDS.kobayashi, pending: true })
    expect(new Set(body.recent.map((item) => item.personId)).size).toBe(body.recent.length)
    expect(body.recent.length).toBeLessThanOrEqual(8)
    expect(body.teams.map((team) => team.group.path)).toEqual(['sales/team-a', 'projects/ceatec'])
    const teamA = body.teams[0]!.members
    expect(teamA.map((item) => item.displayName)).toEqual(['佐藤 花子', '田中 美咲', '鈴木 一郎'].sort((a, b) => a.localeCompare(b, 'ja')))
    expect(teamA.find((item) => item.personId === MOCK_PERSON_IDS.suzuki)).toMatchObject({ pending: true, groups: [MOCK_GROUPS.teamA] })
    expect(body.teams[1]!.members.find((item) => item.personId === MOCK_PERSON_IDS.ito)?.groups).toEqual([MOCK_GROUPS.ceatec])
    expect(body.everyone.total).toBe(8)
    expect(body.everyone.items.map((item) => item.personId)).not.toContain(MOCK_PERSON_IDS.yamada)
    // The parent group's effective members include its subgroups'.
    await as('kato')
    const kato: Recipients = (await call('GET', '/recipients')).body
    expect(kato.teams.map((team) => team.group.path)).toEqual(['sales'])
    expect(kato.teams[0]!.members.map((item) => item.personId)).toContain(MOCK_PERSON_IDS.kobayashi)
    await as('watanabe')
    expect((await call('GET', '/recipients')).body.teams).toEqual([])
    await as('yamada')
    await call('POST', '/__mock/directory', { body: { up: false } })
    const down: Recipients = (await call('GET', '/recipients')).body
    expect(down).toMatchObject({ directoryAvailable: false, teams: [], everyone: { total: 0, items: [] } })
    expect(down.recent.map((item) => item.personId)).toEqual(body.recent.map((item) => item.personId))
  })

  test('cursor paging walks the whole timeline newest first without repeats; filters narrow it', async () => {
    const seen: string[] = []
    let cursor: string | null = null
    do {
      const page: PostPage = (await call('GET', `/posts?limit=7${cursor ? `&cursor=${cursor}` : ''}`)).body
      seen.push(...page.items.map((item) => item.id))
      cursor = page.nextCursor
    } while (cursor)
    expect(seen).toHaveLength(48)
    expect(new Set(seen).size).toBe(48)
    const times = mockState().posts.map((post) => post.createdAt).sort().reverse()
    expect(seen.map((id) => mockState().posts.find((post) => post.id === id)!.createdAt)).toEqual(times)
    const challenge: PostPage = (await call('GET', '/posts?value=challenge&limit=50')).body
    expect(challenge.items.length).toBeGreaterThan(0)
    expect(challenge.items.every((item) => item.values.includes('challenge'))).toBe(true)
    const sent: PostPage = (await call('GET', `/posts?sent=${MOCK_MEMBER_IDS.sato}&limit=50`)).body
    expect(sent.items.every((item) => item.sender.id === MOCK_MEMBER_IDS.sato)).toBe(true)
    const months = new Set(mockState().posts.map((post) => post.monthKey))
    expect(months.size).toBeGreaterThanOrEqual(3)
    expect((await call('GET', '/posts?cursor=%%%')).status).toBe(400)
    expect((await call('GET', '/posts?limit=51')).status).toBe(400)
    expect((await call('GET', '/posts?month=2026-13')).status).toBe(400)
    expect((await call('GET', '/posts?period=2026-W54')).status).toBe(400)
  })

  test('inbox combines received posts and others’ claps, pages newest first, and seen is owner-specific and monotonic', async () => {
    const items: InboxPage['items'] = []
    let cursor: string | null = null
    do {
      const page: InboxPage = (await call('GET', `/inbox?limit=3${cursor ? `&cursor=${cursor}` : ''}`)).body
      items.push(...page.items)
      cursor = page.nextCursor
    } while (cursor)
    expect(new Set(items.map((item) => item.id)).size).toBe(items.length)
    expect(items.map((item) => item.createdAt)).toEqual(items.map((item) => item.createdAt).sort().reverse())
    expect(items.some((item) => item.kind === 'post')).toBe(true)
    expect(items.some((item) => item.kind === 'clap')).toBe(true)
    expect(items.every((item) => item.kind === 'post' ? item.post.recipient.id === MOCK_MEMBER_IDS.yamada : item.actor.id !== MOCK_MEMBER_IDS.yamada && [item.post.sender.id, item.post.recipient.id].includes(MOCK_MEMBER_IDS.yamada))).toBe(true)
    expect((await me()).inbox.unread).toBe(items.filter((item) => item.unread).length)
    expect((await me()).inbox.unread).toBeGreaterThan(0)
    const seen = await call('POST', '/inbox/seen')
    expect(seen.body.unread).toBe(0)
    expect((await me()).inbox.unread).toBe(0)
    expect(((await call('GET', '/inbox?limit=50')).body as InboxPage).items.every((item) => !item.unread)).toBe(true)
    setMockClock(NOW - 60_000)
    expect((await call('POST', '/inbox/seen')).body.seenAt).toBe(seen.body.seenAt)
    setMockClock(NOW + 60_000)
    await as('sato')
    const post = (await send('yamada', 5)).body as Post
    expect((await me()).inbox.unread).toBeGreaterThan(0)
    await as('yamada')
    expect((await me()).inbox.unread).toBe(1)
    expect((await call('GET', '/inbox')).body.items[0]).toMatchObject({ id: post.id, unread: true })
    expect((await call('GET', '/inbox?limit=51')).status).toBe(400)
    expect((await call('GET', '/inbox?cursor=%%%')).status).toBe(400)
  })

  test('post search is a literal case-insensitive substring of message, sender or recipient', async () => {
    const post = (await send('sato', 5, { message: 'Release 100%_Ready' })).body as Post
    expect((await call('GET', '/posts?q=release')).body.items.map((item: Post) => item.id)).toContain(post.id)
    expect((await call('GET', '/posts?q=100%25_')).body.items.map((item: Post) => item.id)).toEqual([post.id])
    expect((await call('GET', '/posts?q=%25not-a-wildcard')).body.items).toEqual([])
    for (const name of ['山田', '佐藤', 'ken watanabe']) {
      const page: PostPage = (await call('GET', `/posts?q=${encodeURIComponent(name)}&limit=50`)).body
      expect(page.items.length).toBeGreaterThan(0)
      expect(page.items.every((item) => [item.sender.displayName, item.recipient.displayName, item.message].some((text) => text.toLowerCase().includes(name.toLowerCase())))).toBe(true)
    }
    expect((await call('GET', `/posts?q=${'x'.repeat(101)}`)).status).toBe(400)
  })

  test('posts expose only the newest three clappers while the clap list keeps everyone', async () => {
    const post = (await send('sato', 5)).body as Post
    for (const member of ['kato', 'ito', 'tanaka', 'nakamura'] as const) {
      await as(member)
      expect((await call('POST', `/posts/${post.id}/claps`)).status).toBe(201)
    }
    const detail: Post = (await call('GET', `/posts/${post.id}`)).body
    expect(detail.clapCount).toBe(4)
    expect(detail.clappers.map((member) => member.id)).toEqual([MOCK_MEMBER_IDS.nakamura, MOCK_MEMBER_IDS.tanaka, MOCK_MEMBER_IDS.ito])
    expect((await call('GET', `/posts/${post.id}/claps`)).body.items).toHaveLength(4)
  })

  test('profiles expose direct Directory groups, all-time partners and values, and six zero-filled months', async () => {
    const profile: MemberProfile = (await call('GET', `/members/${MOCK_MEMBER_IDS.yamada}`)).body
    expect(profile.groups).toEqual([MOCK_GROUPS.teamA, MOCK_GROUPS.ceatec])
    expect(profile.stats.months.map((month) => month.key)).toEqual(['2026-05', '2026-06', '2026-07', '2026-08', '2026-09', '2026-10'])
    expect(profile.stats.months[0]).toEqual({ key: '2026-05', receivedPoints: 0, receivedPosts: 0, sentPosts: 0 })
    for (const partners of Object.values(profile.stats.partners)) {
      expect(partners.length).toBeLessThanOrEqual(5)
      expect(partners.map((partner) => partner.points)).toEqual(partners.map((partner) => partner.points).sort((a, b) => b - a))
    }
    expect(profile.stats.values.map((value) => value.posts)).toEqual(profile.stats.values.map((value) => value.posts).sort((a, b) => b - a))
    expect((await me()).program.values.map((value) => [value.key, value.label])).toEqual([['thanks', '感謝'], ['challenge', '挑戦'], ['teamwork', '協力'], ['learning', '学び'], ['customer', '顧客志向']])
    expect(new Set(mockState().posts.map((post) => post.values.length))).toEqual(new Set([0, 1, 2, 3]))
    await call('POST', '/__mock/directory', { body: { up: false } })
    expect((await call('GET', `/members/${MOCK_MEMBER_IDS.yamada}`)).body.groups).toEqual([])
  })
})
