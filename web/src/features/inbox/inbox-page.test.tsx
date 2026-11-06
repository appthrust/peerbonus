import { beforeEach, describe, expect, test } from 'bun:test'
import type { QueryClient } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import type { InboxPage as InboxPageData, Me } from '../../api/peerbonus'
import { inboxQuery } from '../../api/queries'
import { peerbonusRoutes } from '../../dev/mock-peerbonus'
import { mockGet, mockMe, render } from '../../test/render'
import { InboxPage } from './inbox-page'

beforeEach(async () => {
  const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/reset')
  await peerbonusRoutes(new Request(url, { method: 'POST' }), url)
})

/** The page under a one-route memory router (its cards link), with the inbox answer cached. */
async function renderInbox(me: Me, page: InboxPageData, language: 'ja' | 'en' = 'ja') {
  const root = createRootRoute()
  const route = createRoute({ getParentRoute: () => root, path: '/', component: InboxPage })
  // Plain URLSearchParams search strings, as the app's router (router.tsx) renders them.
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
    parseSearch: (search) => Object.fromEntries(new URLSearchParams(search)),
    stringifySearch: (search) => {
      const text = new URLSearchParams(search as Record<string, string>).toString()
      return text ? `?${text}` : ''
    },
  })
  await router.load()
  const seed = (client: QueryClient) => client.setQueryData(inboxQuery.queryKey, { pages: [page], pageParams: [null] })
  return render(<RouterProvider router={router} />, me, language, seed)
}

describe('inbox page', () => {
  test('items newest first, each with its lead; unread ones carry the dot', async () => {
    const me = await mockMe()
    const page = { ...(await mockGet<InboxPageData>('/inbox?limit=5')), nextCursor: 'next' }
    const unread = page.items.filter((item) => item.unread)
    expect(unread.length).toBeGreaterThan(0)
    expect(page.items.some((item) => !item.unread)).toBe(true)
    const html = await renderInbox(me, page)
    expect(html).toContain('<h1')
    expect(html).toContain('受信箱')
    const first = page.items[0]!
    expect(html).toContain(first.kind === 'clap' ? `${first.actor.displayName} さんが拍手しました` : `${first.actor.displayName} さんから届きました`)
    expect(html).toContain('さんから届きました')
    expect(html.match(/<span class="sr-only">未読: <\/span>/g)).toHaveLength(unread.length)
    expect(html.match(/<article/g)).toHaveLength(page.items.length)
    expect(html).toContain('もっと読む')
  })

  test('items newer than the watermark at open keep the dot after the server marked them read', async () => {
    const me = await mockMe()
    const page = await mockGet<InboxPageData>('/inbox')
    const read = { ...page, items: page.items.map((item) => ({ ...item, unread: false })) }
    const html = await renderInbox(me, read)
    expect(html.match(/<span class="sr-only">未読: <\/span>/g)).toHaveLength(page.items.filter((item) => item.unread).length)
  })

  test('empty: the explanation and a 贈る action that opens the form', async () => {
    const me = await mockMe()
    const html = await renderInbox(me, { items: [], nextCursor: null, seenAt: null })
    expect(html).toContain('まだ何も届いていません')
    expect(html).toContain('href="/?compose=1"')
    expect(html).toContain('感謝を贈る')
    expect(await renderInbox(me, { items: [], nextCursor: null, seenAt: null }, 'en')).toContain('Nothing here yet')
  })
})
