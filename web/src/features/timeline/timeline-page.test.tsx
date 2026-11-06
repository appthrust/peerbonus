import { beforeEach, describe, expect, test } from 'bun:test'
import type { QueryClient } from '@tanstack/react-query'
import type { Me, Post } from '../../api/peerbonus'
import { timelineQuery, type Feed } from '../../api/queries'
import { peerbonusRoutes } from '../../dev/mock-peerbonus'
import { mockMe, mockPosts, render } from '../../test/render'
import type { TimelineFilters } from './filter-bar'
import { shouldRemind, TimelinePage } from './timeline-page'

beforeEach(async () => {
  const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/reset')
  await peerbonusRoutes(new Request(url, { method: 'POST' }), url)
})

const HOUR = 60 * 60 * 1000

/** Caches the first page of `feed` (static markup never fetches). */
const seedFeed = (feed: Feed, items: Post[]) => (client: QueryClient) =>
  client.setQueryData(timelineQuery(feed).queryKey, { pages: [{ items, nextCursor: null }], pageParams: [null] })

const page = (me: Me, filters: TimelineFilters, items: Post[]) =>
  render(<TimelinePage filters={filters} onFiltersChange={() => {}} />, me, 'ja', seedFeed({ kind: 'all', ...filters }, items))

describe('reminder', () => {
  const now = Date.parse('2026-10-25T12:00:00Z')
  const stats = (remaining: number, endsInHours: number) => ({
    key: '2026-W43',
    startsAt: '2026-10-19T00:00:00Z',
    endsAt: new Date(now + endsInHours * HOUR).toISOString(),
    allowance: 400,
    remaining,
    sentPosts: 0,
    sentPoints: 0,
    claps: 0,
  })

  test('only with at least half the budget left and the week ending within two days', () => {
    expect(shouldRemind(stats(200, 47), now)).toBe(true)
    expect(shouldRemind(stats(400, 1), now)).toBe(true)
    expect(shouldRemind(stats(199, 47), now)).toBe(false)
    expect(shouldRemind(stats(400, 49), now)).toBe(false)
    expect(shouldRemind(stats(400, -1), now)).toBe(false)
  })

  test('the banner names what is left and offers 贈る', async () => {
    const me = await mockMe()
    const soon = { ...me, stats: { ...me.stats, period: { ...me.stats.period, remaining: 300, allowance: 400, endsAt: new Date(Date.now() + 5 * HOUR).toISOString() } } }
    const html = page(soon, {}, await mockPosts())
    expect(html).toContain('今週あと 300 pt。')
    expect(html).toContain('に消えます')
    const quiet = { ...me, stats: { ...me.stats, period: { ...me.stats.period, remaining: 10, allowance: 400, endsAt: new Date(Date.now() + 5 * HOUR).toISOString() } } }
    expect(page(quiet, {}, await mockPosts())).not.toContain('に消えます')
  })
})

describe('timeline page', () => {
  test('filter bar: すべて / 自分 links, every value as a chip, months and search; then the launcher and the feed', async () => {
    const me = await mockMe()
    const posts = await mockPosts()
    const html = page(me, {}, posts)
    expect(html).toContain('href="/?mine=1"')
    for (const value of me.program.values) expect(html).toContain(`>${value.label}</span>`)
    expect(html).toContain('すべての月')
    expect(html).toContain('placeholder="メッセージや名前で検索"')
    expect(html.indexOf('role="search"')).toBeLessThan(html.indexOf(`${posts[0].recipient.displayName}</a> さんへ`))
  })

  test('a selected value chip is filled and links back to all values; a retired key stays selectable', async () => {
    const me = await mockMe()
    const html = page(me, { value: 'retired', mine: true }, [])
    expect(html).toContain('>#retired</span>')
    expect(html).toContain('href="/?mine=1"')
    expect(html).toContain('bg-stone-800 text-white')
  })

  test('empty states carry the next action', async () => {
    const me = await mockMe()
    const none = page(me, {}, [])
    expect(none).toContain('まだ投稿がありません')
    expect(none).toContain('最初の投稿を贈る')
    const filtered = page(me, { q: 'zzz' }, [])
    expect(filtered).toContain('条件に合う投稿はありません')
    expect(filtered).toContain('条件をゆるめる')
    expect(filtered).not.toContain('最初の投稿を贈る')
  })
})
