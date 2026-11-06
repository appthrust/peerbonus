import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import type { ReactNode } from 'react'
import { timelineQuery, type Feed } from '../../api/queries'
import { useText } from '../../i18n'
import { ActionError, Button, cn, CLOCK_TICK_MS, LoadFailed, Loading, useNow } from '../../ui'
import { PostCard } from '../posts/post-card'

/**
 * Posts of `feed`, newest first, one more cursor page per もっと読む; `empty`
 * (which carries the next action) when there are none. A new filter keeps the
 * previous posts on screen, dimmed, until its first page lands, so typing a
 * search never blanks the page. `highlightedId` glows a post just sent.
 */
export function PostList({ feed, empty, highlightedId, now }: { feed: Feed; empty: ReactNode; highlightedId?: string | null; now?: number }) {
  const { t } = useText()
  // Callers that own a page clock pass `now`; otherwise the list keeps its own, moving every minute.
  const ownClock = useNow(now === undefined ? CLOCK_TICK_MS : undefined)
  const { data, error, isPending, isError, isPlaceholderData, refetch, fetchNextPage, hasNextPage, isFetchingNextPage, isFetchNextPageError } = useInfiniteQuery({
    ...timelineQuery(feed),
    placeholderData: keepPreviousData,
  })
  if (isPending) return <Loading />
  if (isError && !data) return <LoadFailed error={error} onRetry={() => void refetch()} />
  const posts = data.pages.flatMap((page) => page.items)
  if (posts.length === 0 && !isPlaceholderData) return <>{empty}</>
  return (
    <div aria-busy={isPlaceholderData || undefined} className={cn('transition-opacity', isPlaceholderData ? 'opacity-60' : undefined)}>
      <ul className="grid gap-3">
        {posts.map((post) => (
          <li key={post.id}>
            <PostCard post={post} highlighted={post.id === highlightedId} now={now ?? ownClock} />
          </li>
        ))}
      </ul>
      {isFetchNextPageError ? <ActionError error={error} className="mt-4" /> : null}
      {hasNextPage ? (
        <div className="mt-6 flex justify-center">
          <Button icon={<ChevronDown />} loading={isFetchingNextPage} disabled={isPlaceholderData} onClick={() => void fetchNextPage()}>
            {t('peerbonus.common.loadMore')}
          </Button>
        </div>
      ) : data.pages.length > 1 ? (
        <p className="mt-6 text-center text-xs text-stone-500">{t('peerbonus.timeline.end')}</p>
      ) : null}
    </div>
  )
}
