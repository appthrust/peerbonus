// Page routes: the timeline with the compose launcher, a post, a member, `/me`
// (forwards to the caller's member page), the inbox and the admin section
// (./admin-pages).
import { createRoute, redirect } from '@tanstack/react-router'
import { useCallback } from 'react'
import { clapsQuery, meQuery, memberQuery, postQuery, recipientsQuery, timelineQuery } from '../api/queries'
import { inboxRoute } from '../features/inbox/inbox-route'
import { MemberPage } from '../features/members/member-page'
import { PostPage } from '../features/posts/post-page'
import type { TimelineFilters } from '../features/timeline/filter-bar'
import { TimelinePage } from '../features/timeline/timeline-page'
import { isMonthKey } from '../model/period'
import { VALUE_KEY } from '../model/post'
import { adminRoutes } from './admin-pages'
import type { MemberTab } from './paths'
import { notFoundOn404, rootRoute } from './root'

/** The server's limit on `q`. */
const MAX_QUERY_LENGTH = 100

type TimelineSearch = { value?: string; month?: string; q?: string; mine?: '1'; to?: string; compose?: '1' }

/**
 * `?value=<key>`, `?month=YYYY-MM`, `?q=<text>` and `?mine=1` narrow the feed; anything malformed is dropped.
 * `?to=<personId>` opens the launcher with that recipient (「贈る」 on posts and member pages) and
 * `?compose=1` opens it empty (the inbox's 贈る); the page drops both once the launcher took them.
 */
function timelineSearch(search: Record<string, unknown>): TimelineSearch {
  const q = typeof search.q === 'string' ? search.q.trim().slice(0, MAX_QUERY_LENGTH) : ''
  return {
    ...(typeof search.value === 'string' && VALUE_KEY.test(search.value) ? { value: search.value } : {}),
    ...(typeof search.month === 'string' && isMonthKey(search.month) ? { month: search.month } : {}),
    ...(q ? { q } : {}),
    ...(search.mine === '1' ? { mine: '1' as const } : {}),
    ...(typeof search.to === 'string' && search.to.length > 0 && search.to.length <= 128 ? { to: search.to } : {}),
    ...(search.compose === '1' ? { compose: '1' as const } : {}),
  }
}

function timelineFilters(search: TimelineSearch): TimelineFilters {
  return { value: search.value, month: search.month, q: search.q, mine: search.mine === '1' }
}

export const timelineRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  validateSearch: timelineSearch,
  // No loaderDeps on purpose: a new filter is the same page, so the router never swaps it for the
  // pending screen while someone types a search; the list keeps the previous posts until the new ones land.
  loader: ({ context, location }) => {
    // The picker's people load alongside the timeline; a failure shows in the picker, not as a page error.
    void context.queryClient.prefetchQuery(recipientsQuery)
    const filters = timelineFilters(timelineSearch(location.search as Record<string, unknown>))
    return context.queryClient.ensureInfiniteQueryData(timelineQuery({ kind: 'all', ...filters }))
  },
  component: function TimelineRoute() {
    const search = timelineRoute.useSearch()
    const navigate = timelineRoute.useNavigate()
    const filters = timelineFilters(search)
    const onFiltersChange = useCallback(
      (next: TimelineFilters, options?: { replace?: boolean }) =>
        void navigate({
          search: { value: next.value, month: next.month, q: next.q, mine: next.mine ? '1' : undefined },
          replace: options?.replace,
        }),
      [navigate]
    )
    // Once the launcher took `to` / `compose`, they leave the URL, so a reload or another 「贈る」 starts fresh.
    const dropTransient = useCallback(() => void navigate({ search: (previous) => ({ ...previous, to: undefined, compose: undefined }), replace: true }), [navigate])
    return (
      <TimelinePage
        filters={filters}
        to={search.to}
        compose={search.compose === '1'}
        onFiltersChange={onFiltersChange}
        onPreselected={dropTransient}
        onComposeHandled={dropTransient}
      />
    )
  },
})

export const postRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/posts/$postId',
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(postQuery(params.postId)).catch(notFoundOn404),
      context.queryClient.ensureQueryData(clapsQuery(params.postId)).catch(notFoundOn404),
    ])
  },
  component: function PostRoute() {
    const { postId } = postRoute.useParams()
    return <PostPage key={postId} postId={postId} />
  },
})

type MemberSearch = { tab?: 'sent' }

export const memberRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/members/$memberId',
  // `?tab=sent` is the 贈った tab; anything else is もらった.
  validateSearch: (search: Record<string, unknown>): MemberSearch => (search.tab === 'sent' ? { tab: 'sent' } : {}),
  loaderDeps: ({ search }) => ({ tab: (search.tab ?? 'received') as MemberTab }),
  loader: async ({ context, params, deps }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(memberQuery(params.memberId)).catch(notFoundOn404),
      context.queryClient.ensureInfiniteQueryData(timelineQuery({ kind: deps.tab, memberId: params.memberId })),
    ])
  },
  component: function MemberRoute() {
    const { memberId } = memberRoute.useParams()
    const { tab } = memberRoute.useSearch()
    return <MemberPage key={memberId} memberId={memberId} tab={tab ?? 'received'} />
  },
})

export const meRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/me',
  // The root already loaded `GET /me`; this only forwards to the caller's member page, which adds this week.
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQuery)
    throw redirect({ to: '/members/$memberId', params: { memberId: me.member.id }, replace: true })
  },
})

export const routeTree = rootRoute.addChildren([timelineRoute, postRoute, memberRoute, meRoute, inboxRoute, ...adminRoutes])
