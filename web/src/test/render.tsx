// Test-only helpers: render a screen part with the real copy, a query cache
// already holding `GET /me` from the in-repo mock, and a bare router at `/` so
// kit links (HrefLink) render their hrefs.
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterContextProvider } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import type { Me, Post, PostPage, Recipients } from '../api/peerbonus'
import { meQuery } from '../api/queries'
import { peerbonusRoutes } from '../dev/mock-peerbonus'
import { I18nProvider } from '../i18n'

export async function mockGet<T>(path: string): Promise<T> {
  const url = new URL(`/api/peerbonus/v1${path}`, 'http://peerbonus.test')
  const response = await peerbonusRoutes(new Request(url), url)
  if (!response?.ok) throw new Error(`mock GET ${path} failed: ${response?.status}`)
  return (await response.json()) as T
}

export const mockMe = () => mockGet<Me>('/me')
export const mockPosts = async () => (await mockGet<PostPage>('/posts?limit=50')).items as Post[]
export const mockRecipients = () => mockGet<Recipients>('/recipients')

/**
 * Static markup of `node` for `me`, in Japanese unless `language` says otherwise. `seed` puts more
 * answers in the cache first (e.g. `GET /recipients`), since static markup never fetches.
 */
export function render(node: ReactNode, me: Me, language: 'ja' | 'en' = 'ja', seed?: (client: QueryClient) => void): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  client.setQueryData(meQuery.queryKey, me)
  seed?.(client)
  // Plain `?a=b` search params, as src/router.tsx configures them (not TanStack's JSON default).
  const router = createRouter({
    routeTree: createRootRoute(),
    history: createMemoryHistory({ initialEntries: ['/'] }),
    parseSearch: (search) => Object.fromEntries(new URLSearchParams(search)),
    stringifySearch: (search) => {
      const text = new URLSearchParams(Object.entries(search).filter((entry): entry is [string, string] => typeof entry[1] === 'string' && entry[1] !== '')).toString()
      return text ? `?${text}` : ''
    },
  })
  return renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}>
        <I18nProvider language={language}>{node}</I18nProvider>
      </RouterContextProvider>
    </QueryClientProvider>
  )
}
