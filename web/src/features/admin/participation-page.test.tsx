import { beforeEach, describe, expect, test } from 'bun:test'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { renderToStaticMarkup } from 'react-dom/server'
import type { Participation, StatementSummary } from '../../api/peerbonus'
import { meQuery } from '../../api/queries'
import '../../dev/mock-admin'
import { peerbonusRoutes } from '../../dev/mock-peerbonus'
import { I18nProvider } from '../../i18n'
import { mockGet, mockMe } from '../../test/render'
import { isActive, ParticipationPage } from './participation-page'
import { adminMembersQuery, statementsQuery } from './queries'

beforeEach(async () => {
  const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/reset')
  await peerbonusRoutes(new Request(url, { method: 'POST' }), url)
})

/** Static markup of the participation page for `data`, inside a memory router (the rows carry profile links). */
async function renderPage(data: Participation): Promise<string> {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(meQuery.queryKey, await mockMe())
  client.setQueryData(adminMembersQuery(data.month).queryKey, data)
  const statements = await mockGet<{ items: StatementSummary[] }>('/admin/statements')
  client.setQueryData(statementsQuery.queryKey, statements.items)
  const rootRoute = createRootRoute({
    component: () => <ParticipationPage month={data.month} currentMonth={data.month} onMonthChange={() => {}} />,
  })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory({ initialEntries: ['/admin/members'] }) })
  await router.load()
  return renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <I18nProvider language="ja">
        <RouterProvider router={router} />
      </I18nProvider>
    </QueryClientProvider>
  )
}

describe('participation page', () => {
  test('every Directory person is a row; someone who is not a member yet has no profile link', async () => {
    const data = await mockGet<Participation>('/admin/members')
    const outsider = data.items.find((row) => row.member === null)
    const member = data.items.find((row) => row.member !== null && isActive(row))
    expect(outsider).toBeDefined()
    expect(member).toBeDefined()
    const html = await renderPage(data)
    expect(html).toContain(`>${outsider!.displayName}</span>`)
    expect(html).not.toContain(`>${outsider!.displayName}</a>`)
    expect(html).toContain('まだ参加していません')
    expect(html).toContain(`href="/members/${member!.member!.id}"`)
    expect(html).toContain(`>${member!.displayName}</a>`)
  })

  test('summary cards show people, senders, recipients and the participation rate', async () => {
    const data: Participation = {
      month: '2026-10',
      directoryAvailable: true,
      summary: { people: 8, senders: 3, recipients: 4, active: 6 },
      items: [],
    }
    const html = await renderPage(data)
    expect(html).toContain('参加率')
    expect(html).toContain('>75</span>')
    expect(html).toContain('6 人が参加・2 人がまだ')
    expect(html).not.toContain('Directory に接続できません')
  })

  test('without Directory the page says only active members are listed', async () => {
    const data = await mockGet<Participation>('/admin/members')
    const html = await renderPage({ ...data, directoryAvailable: false, items: data.items.filter((row) => row.member !== null) })
    expect(html).toContain('Directory に接続できません')
    expect(html).not.toContain('まだ参加していません')
  })

  test('activity counts gives, receipts, claps and a non-zero balance change', () => {
    const quiet = { personId: 'p', displayName: 'x', email: 'x@example.com', pending: false, avatarUrl: null, member: null, sentPosts: 0, sentPoints: 0, receivedPosts: 0, receivedPoints: 0, clapsGiven: 0 }
    expect(isActive(quiet)).toBe(false)
    expect(isActive({ ...quiet, clapsGiven: 1 })).toBe(true)
    expect(isActive({ ...quiet, receivedPoints: -10 })).toBe(true)
  })
})
