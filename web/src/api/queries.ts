// Read-model loading (React Query). Every query is a `queryOptions()` /
// `infiniteQueryOptions()` factory so route loaders (`ensureQueryData`) and
// components share one key and fetcher.
import { infiniteQueryOptions, QueryClient, queryOptions, type InfiniteData } from '@tanstack/react-query'
import { peerbonusApi, PeerbonusApiError, type InboxPage, type Me, type Member, type MemberProfile, type Post, type PostPage } from './peerbonus'

/** Retries for a call nothing answered (rollout gap, network blip). */
const TRANSIENT_RETRIES = 3

/** Any answer with a status (4xx, coded 5xx) is final; unanswered calls are retried with backoff. */
export const retryQuery = (failureCount: number, error: unknown) =>
  error instanceof PeerbonusApiError && !error.transient ? false : failureCount < TRANSIENT_RETRIES

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: retryQuery, retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 8000), staleTime: 10_000 },
  },
})

/** Every peerbonus key starts with this, so one invalidation refreshes everything after a write. */
export const PEERBONUS_KEY = ['peerbonus'] as const

/** A write (send, clap, delete, settings) moves balances, timelines and stats; everything lives under PEERBONUS_KEY. */
export const invalidatePeerbonus = (client: QueryClient) => client.invalidateQueries({ queryKey: PEERBONUS_KEY })

export const meQuery = queryOptions({ queryKey: [...PEERBONUS_KEY, 'me'], queryFn: () => peerbonusApi.me(), staleTime: 60_000 })

/** `GET /config`: fixed for the server's lifetime. */
export const configQuery = queryOptions({ queryKey: [...PEERBONUS_KEY, 'config'], queryFn: () => peerbonusApi.config(), staleTime: Infinity })

/** Timeline page size (the server's default). */
export const PAGE_SIZE = 20

/**
 * Which posts a list shows: everyone's, narrowed by value key, month (`YYYY-MM`), text (`q`) and
 * `mine` (sent or received by the caller); or one member's sent / received.
 */
export type Feed = { kind: 'all'; value?: string; month?: string; q?: string; mine?: boolean } | { kind: 'sent' | 'received'; memberId: string }

const POSTS_KEY = [...PEERBONUS_KEY, 'posts'] as const
const TIMELINE_KEY = [...POSTS_KEY, 'timeline'] as const

/** The feed with unset filters dropped, so `{kind:'all'}` and `{kind:'all', q:''}` share one cache entry. */
function normalizeFeed(feed: Feed): Feed {
  if (feed.kind !== 'all') return { kind: feed.kind, memberId: feed.memberId }
  const q = feed.q?.trim()
  return {
    kind: 'all',
    ...(feed.value ? { value: feed.value } : {}),
    ...(feed.month ? { month: feed.month } : {}),
    ...(q ? { q } : {}),
    ...(feed.mine ? { mine: true } : {}),
  }
}

/** Posts newest first, one cursor page per "もっと読む". `mine` resolves the caller through the cached `me`. */
export const timelineQuery = (feed: Feed) => {
  const normalized = normalizeFeed(feed)
  return infiniteQueryOptions({
    queryKey: [...TIMELINE_KEY, normalized],
    queryFn: async ({ pageParam, client }) => {
      const all = normalized.kind === 'all' ? normalized : undefined
      const member = all?.mine ? (await client.ensureQueryData(meQuery)).member.id : undefined
      return peerbonusApi.posts.list({
        limit: PAGE_SIZE,
        cursor: pageParam,
        member,
        value: all?.value,
        month: all?.month,
        q: all?.q,
        sent: normalized.kind === 'sent' ? normalized.memberId : undefined,
        received: normalized.kind === 'received' ? normalized.memberId : undefined,
      })
    },
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.nextCursor,
  })
}

/** Inbox page size. */
export const INBOX_PAGE_SIZE = 20

/** Received posts and claps on the caller's posts, newest first; one cursor page per "もっと読む". */
export const inboxQuery = infiniteQueryOptions({
  queryKey: [...PEERBONUS_KEY, 'inbox'],
  queryFn: ({ pageParam }) => peerbonusApi.inbox.list({ limit: INBOX_PAGE_SIZE, cursor: pageParam }),
  initialPageParam: null as string | null,
  getNextPageParam: (page: InboxPage) => page.nextCursor,
})

/** Admin insights for one month (`YYYY-MM`). */
export const insightsQuery = (month: string) =>
  queryOptions({ queryKey: [...PEERBONUS_KEY, 'admin', 'insights', month], queryFn: () => peerbonusApi.admin.insights(month) })

/** The admin network map for one month, optionally narrowed to a Directory group (`''` = everyone). */
export const networkQuery = (month: string, group = '') =>
  queryOptions({ queryKey: [...PEERBONUS_KEY, 'admin', 'network', month, group], queryFn: () => peerbonusApi.admin.network(month, group || undefined) })

export const postQuery = (postId: string) => queryOptions({ queryKey: [...POSTS_KEY, 'one', postId], queryFn: () => peerbonusApi.posts.get(postId) })

export const clapsQuery = (postId: string) => queryOptions({ queryKey: [...POSTS_KEY, 'claps', postId], queryFn: () => peerbonusApi.posts.claps(postId) })

export const memberQuery = (memberId: string) =>
  queryOptions({ queryKey: [...PEERBONUS_KEY, 'members', memberId], queryFn: () => peerbonusApi.members.get(memberId) })

/** The recipient picker's first view (recent, teams, everyone); the server caches Directory for a minute too. */
export const recipientsQuery = queryOptions({ queryKey: [...PEERBONUS_KEY, 'recipients'], queryFn: () => peerbonusApi.recipients(), staleTime: 60_000 })

/** How many people one server search returns (the server's maximum). */
export const CANDIDATE_LIMIT = 20

/** Server-side refinement of the picker's "everyone" list; never cached long (Directory is the source). */
export const candidatesQuery = (q: string) =>
  queryOptions({ queryKey: [...PEERBONUS_KEY, 'candidates', q], queryFn: () => peerbonusApi.members.search(q, CANDIDATE_LIMIT), staleTime: 30_000 })

/**
 * Puts the server's answer for one post into every cached timeline page and its
 * detail, so a clap shows at once, before the invalidation refetch lands.
 */
export function storePost(client: QueryClient, post: Post) {
  client.setQueryData(postQuery(post.id).queryKey, post)
  client.setQueriesData<InfiniteData<PostPage>>({ queryKey: TIMELINE_KEY }, (data) =>
    data ? { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((item) => (item.id === post.id ? post : item)) })) } : data
  )
}

/** Whether a post the caller just sent belongs in this feed (`mine`: the caller is its sender). */
function feedShows(feed: Feed, post: Post): boolean {
  if (feed.kind !== 'all') return (feed.kind === 'sent' ? post.sender : post.recipient).id === feed.memberId
  if (feed.value && !post.values.includes(feed.value)) return false
  if (feed.month && post.month !== feed.month) return false
  if (feed.q) {
    const q = feed.q.toLowerCase()
    if (![post.message, post.sender.displayName, post.recipient.displayName].some((text) => text.toLowerCase().includes(q))) return false
  }
  return true
}

/**
 * Puts a post the caller just sent at the top of the first page of every cached timeline it belongs
 * to (once: a post already there is replaced in place), so it shows before the refetch lands.
 */
export function prependPost(client: QueryClient, post: Post) {
  client.setQueryData(postQuery(post.id).queryKey, post)
  for (const query of client.getQueryCache().findAll({ queryKey: TIMELINE_KEY })) {
    const feed = query.queryKey[TIMELINE_KEY.length] as Feed | undefined
    if (!feed || !feedShows(feed, post)) continue
    client.setQueryData<InfiniteData<PostPage>>(query.queryKey, (data) => {
      if (!data || data.pages.length === 0) return data
      if (data.pages.some((page) => page.items.some((item) => item.id === post.id))) {
        return { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((item) => (item.id === post.id ? post : item)) })) }
      }
      const [first, ...rest] = data.pages
      return { ...data, pages: [{ ...first, items: [post, ...first.items] }, ...rest] }
    })
  }
}

/** Sets the cached `me` unread count (the inbox page zeroes it once the server moved the watermark). */
export function setInboxUnread(client: QueryClient, unread: number) {
  client.setQueryData<Me>(meQuery.queryKey, (me) => (me ? { ...me, inbox: { unread } } : me))
}

/**
 * A Member with this Directory `personId` from any loaded member page, post or timeline page, so
 * 「贈る」 on those screens can fill the form even for someone the picker's lists do not include.
 */
export function cachedMember(client: QueryClient, personId: string): Member | undefined {
  const members: Member[] = []
  for (const [, profile] of client.getQueriesData<MemberProfile>({ queryKey: [...PEERBONUS_KEY, 'members'] })) if (profile) members.push(profile.member)
  for (const [, post] of client.getQueriesData<Post>({ queryKey: [...POSTS_KEY, 'one'] })) if (post) members.push(post.sender, post.recipient)
  for (const [, data] of client.getQueriesData<InfiniteData<PostPage>>({ queryKey: TIMELINE_KEY })) {
    for (const page of data?.pages ?? []) for (const post of page.items) members.push(post.sender, post.recipient)
  }
  return members.find((member) => member.personId === personId)
}
