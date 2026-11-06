import { createRoute } from '@tanstack/react-router'
import { inboxQuery } from '../../api/queries'
import { rootRoute } from '../../routes/root'
import { InboxPage } from './inbox-page'

/** `/inbox`: always read fresh, so the unread markers reflect the watermark before this visit moves it. */
export const inboxRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/inbox',
  loader: ({ context }) => context.queryClient.fetchInfiniteQuery({ ...inboxQuery, staleTime: 0 }),
  component: InboxPage,
})
