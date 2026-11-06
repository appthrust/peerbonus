import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { renderToStaticMarkup } from 'react-dom/server'
import type { Network, StatementSummary } from '../../../api/peerbonus'
import { meQuery, networkQuery } from '../../../api/queries'
import '../../../dev/mock-admin'
import { MOCK_GROUPS, MOCK_PERSON_IDS, peerbonusRoutes, setMockClock } from '../../../dev/mock-peerbonus'
import { I18nProvider } from '../../../i18n'
import { mockGet, mockMe } from '../../../test/render'
import { statementsQuery } from '../queries'
import { NetworkPage, networkSearch } from './network-page'

// 2026-10-20 12:00 in Asia/Tokyo, as the admin mock tests pin it.
const NOW = Date.parse('2026-10-20T03:00:00Z')
const MONTH = '2026-10'

beforeEach(async () => {
  setMockClock(NOW)
  const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/reset')
  await peerbonusRoutes(new Request(url, { method: 'POST' }), url)
})

afterAll(() => setMockClock(null))

/** Static markup of the page for `data` (cached under `group`'s query only), inside a memory router. */
async function renderPage(data: Network, group = ''): Promise<string> {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(meQuery.queryKey, await mockMe())
  client.setQueryData(networkQuery(data.month, group).queryKey, data)
  const statements = await mockGet<{ items: StatementSummary[] }>('/admin/statements')
  client.setQueryData(statementsQuery.queryKey, statements.items)
  const rootRoute = createRootRoute({
    component: () => <NetworkPage month={data.month} group={group} currentMonth={MONTH} onChange={() => {}} />,
  })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory({ initialEntries: ['/admin/network'] }) })
  await router.load()
  return renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <I18nProvider language="ja">
        <RouterProvider router={router} />
      </I18nProvider>
    </QueryClientProvider>
  )
}

const withSignal = (data: Network, signal: string) => data.nodes.filter((node) => node.signals.includes(signal as never))

describe('network page', () => {
  test('signal cards name the people behind each signal, with the echoed numbers', async () => {
    const data = await mockGet<Network>(`/admin/network?month=${MONTH}`)
    const [concentrated] = withSignal(data, 'concentrated')
    const [declining] = withSignal(data, 'declining').filter((node) => node.memberId !== null)
    const few = data.nodes.filter((node) => node.signals.includes('isolated') || node.signals.includes('fewConnections'))
    expect(concentrated).toBeDefined()
    expect(declining).toBeDefined()
    expect(few.length).toBeGreaterThan(5)
    const html = await renderPage(data)
    for (const title of ['集中', '利用の急減', 'つながりが少ない', '未拍手']) expect(html).toContain(`>${title}</h3>`)
    expect(html).toContain(`href="/members/${concentrated!.memberId}"`)
    expect(html).toContain(`${concentrated!.receivedPosts} 件・${Math.round(concentrated!.share * 100)}%`)
    expect(html).toContain(`>${declining!.displayName}</a>`)
    expect(html).toContain(`${declining!.decline!.previous} → ${declining!.decline!.recent} 回`)
    expect(html).toContain(`+${few.length - 5} 人`)
    expect(html).toContain('つながりなし')
    // The rules read the response's thresholds.
    expect(html).toContain('今月の投稿の 20% 以上（5 件以上）')
    expect(html).toContain('今月つながった相手が 2 人以下')
  })

  test('card rules follow the thresholds the server echoes', async () => {
    const data = await mockGet<Network>(`/admin/network?month=${MONTH}`)
    const html = await renderPage({ ...data, thresholds: { ...data.thresholds, concentrationShare: 0.3, concentrationPosts: 7, fewConnections: 3, declineRatio: 0.4, declineMinimum: 6 } })
    expect(html).toContain('今月の投稿の 30% 以上（7 件以上）')
    expect(html).toContain('今月つながった相手が 3 人以下')
    expect(html).toContain('（6 回以上）の 40% 以下')
  })

  test('the graph draws every person as a focusable node; members link to their page, the viewer is outlined', async () => {
    const data = await mockGet<Network>(`/admin/network?month=${MONTH}`)
    const html = await renderPage(data)
    expect(html).toContain(`aria-label="${data.nodes.length} 人のつながり"`)
    for (const node of data.nodes) expect(html).toContain(`data-node="${node.personId}"`)
    const outsider = data.nodes.find((node) => node.memberId === null)!
    expect(html).toMatch(new RegExp(`<g (?=[^>]*data-node="${outsider.personId}")(?=[^>]*tabindex="0")(?=[^>]*role="img")`))
    const member = data.nodes.find((node) => node.memberId !== null)!
    expect(html).toContain(`<a href="/members/${member.memberId}" data-node="${member.personId}"`)
    expect(html).toContain('stroke-width="3.5"')
    expect(html).toContain('今週のやりとり')
  })

  test('a group narrows the page to that group’s query, and the picker shows it', async () => {
    const team = MOCK_GROUPS.teamA
    const everyone = await mockGet<Network>(`/admin/network?month=${MONTH}`)
    const data = await mockGet<Network>(`/admin/network?month=${MONTH}&group=${team.id}`)
    expect(data.group).toEqual(team)
    expect(data.nodes.length).toBeLessThan(everyone.nodes.length)
    // Only the group's query is cached: rendering proves the page reads networkQuery(month, group).
    const html = await renderPage(data, team.id)
    expect(html).toContain(`<option value="${team.id}" selected="">${team.name}</option>`)
    expect(html).toContain('<option value="">すべて</option>')
    const outside = everyone.nodes.find((node) => !data.nodes.some((inside) => inside.personId === node.personId))!
    expect(html).not.toContain(`data-node="${outside.personId}"`)
  })

  test('the URL keeps only a non-current month and a chosen group', () => {
    expect(networkSearch(MONTH, '', MONTH)).toEqual({})
    expect(networkSearch('2026-09', '', MONTH)).toEqual({ month: '2026-09' })
    expect(networkSearch(MONTH, 'g-sales', MONTH)).toEqual({ group: 'g-sales' })
    expect(networkSearch('2026-08', 'g-sales', MONTH)).toEqual({ month: '2026-08', group: 'g-sales' })
  })

  test('a month without posts or claps is an empty state, not an empty graph', async () => {
    const data = await mockGet<Network>('/admin/network?month=2026-01')
    expect(data.summary.posts + data.summary.claps).toBe(0)
    const html = await renderPage(data)
    expect(html).toContain('2026年1月の投稿や拍手はまだありません')
    expect(html).not.toContain('data-node=')
    expect(html).not.toContain('>集中</h3>')
  })

  test('without Directory the page says so and offers no group picker', async () => {
    const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/directory')
    await peerbonusRoutes(new Request(url, { method: 'POST', body: JSON.stringify({ up: false }) }), url)
    const data = await mockGet<Network>(`/admin/network?month=${MONTH}&group=${MOCK_GROUPS.teamA.id}`)
    expect(data).toMatchObject({ directoryAvailable: false, group: null, groups: [] })
    // "Members only" as participation degrades: people who never sent or received (no Member) drop out.
    expect(data.nodes.every((node) => node.memberId !== null)).toBe(true)
    expect(data.nodes.some((node) => node.personId === MOCK_PERSON_IDS.kobayashi)).toBe(false)
    expect(data.nodes.some((node) => node.personId === 'p-net-26')).toBe(false)
    expect(data.summary.people).toBe(data.nodes.length)
    const html = await renderPage(data)
    expect(html).toContain('Directory に接続できません')
    expect(html).not.toContain('すべて</option>')
  })
})

describe('network mock', () => {
  test('answers the contract invariants N1–N3 and the signal rules', async () => {
    const data = await mockGet<Network>(`/admin/network?month=${MONTH}`)
    const ids = data.nodes.map((node) => node.personId)
    expect(new Set(ids).size).toBe(ids.length)
    for (const edge of data.edges) {
      expect(edge.from < edge.to).toBe(true)
      expect(ids).toContain(edge.from)
      expect(ids).toContain(edge.to)
    }
    expect(data.summary.connected + data.summary.isolated).toBe(data.summary.people)
    expect(data.summary.people).toBe(40)
    expect(data.recentSince).toBe('2026-10-18T15:00:00.000Z')
    expect(data.groups.map((group) => group.name)).toEqual(['CEATEC 出展', '営業1課', '営業2課', '営業部'])
    expect(withSignal(data, 'isolated').length).toBeGreaterThanOrEqual(5)
    expect(withSignal(data, 'concentrated').length).toBe(1)
    expect(withSignal(data, 'declining').length).toBeGreaterThanOrEqual(1)
    expect(withSignal(data, 'unclapped').length).toBeGreaterThanOrEqual(1)
    expect(Math.max(...data.nodes.map((node) => node.connections))).toBeGreaterThanOrEqual(data.thresholds.manyConnections)
    expect(data.nodes.find((node) => node.personId === MOCK_PERSON_IDS.kobayashi)).toMatchObject({ memberId: null, connections: 0, signals: ['isolated'] })
  })

  test('a past month reads up to its end and has no "this week"; an unknown group is 404', async () => {
    const september = await mockGet<Network>('/admin/network?month=2026-09')
    expect(september.recentSince).toBeNull()
    expect(september.asOf).toBe('2026-09-30T14:59:59.999Z')
    const url = new URL(`http://peerbonus.test/api/peerbonus/v1/admin/network?month=${MONTH}&group=g-gone`)
    const response = await peerbonusRoutes(new Request(url), url)
    expect(response?.status).toBe(404)
    expect(await response?.json()).toMatchObject({ error: 'group_not_found' })
  })
})
