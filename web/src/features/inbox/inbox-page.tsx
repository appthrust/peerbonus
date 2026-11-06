import { useQueryClient, useSuspenseInfiniteQuery } from '@tanstack/react-query'
import { ChevronDown, Gift, Inbox } from 'lucide-react'
import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { peerbonusApi, type InboxItem } from '../../api/peerbonus'
import { inboxQuery, setInboxUnread } from '../../api/queries'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { useSession } from '../../shell/use-session'
import { ActionError, Avatar, Button, CLOCK_TICK_MS, cn, EmptyState, formatDateTime, formatRelativeTime, LinkButton, PageHeader, useDocumentTitle, useNow } from '../../ui'
import { PostCard } from '../posts/post-card'

/**
 * `/inbox`: posts to the caller and claps on their posts, newest first, each under a one-line
 * lead (who did what, when). Opening the page moves the server's watermark (and zeroes the header
 * count) once the first page is on screen; the items that were unread at that moment keep their dot.
 */
export function InboxPage() {
  const { t } = useText()
  const { me } = useSession()
  const client = useQueryClient()
  useDocumentTitle(t('peerbonus.nav.inbox'))
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isFetchNextPageError, error } = useSuspenseInfiniteQuery(inboxQuery)
  const items = data.pages.flatMap((page) => page.items)
  // The watermark as it was when the page opened: later answers come back read.
  const [openedSeenAt] = useState(() => data.pages[0]?.seenAt ?? null)
  // One page clock for every relative time, ticking so a page left open stays current.
  const now = useNow(CLOCK_TICK_MS)
  const marked = useRef(false)

  const markSeen = useEffectEvent(() => {
    if (me.inbox.unread === 0 && !items.some((item) => item.unread)) return
    // A failure leaves the count as it was; the next visit tries again.
    void peerbonusApi.inbox.markSeen().then(
      () => setInboxUnread(client, 0),
      () => {}
    )
  })
  // Once, for the page as it opened.
  useEffect(() => {
    if (marked.current) return
    marked.current = true
    markSeen()
  }, [])

  const wasUnread = (item: InboxItem) => item.unread || openedSeenAt === null || Date.parse(item.createdAt) > Date.parse(openedSeenAt)

  return (
    <div className="mx-auto max-w-2xl">
      <PageHeader title={t('peerbonus.nav.inbox')} />
      {items.length === 0 ? (
        <EmptyState
          icon={<Inbox />}
          title={t('peerbonus.inbox.empty.title')}
          body={t('peerbonus.inbox.empty.body')}
          action={
            <LinkButton href={paths.compose} tone="primary" icon={<Gift />}>
              {t('peerbonus.composer.title')}
            </LinkButton>
          }
        />
      ) : (
        <>
          <ul className="grid gap-5">
            {items.map((item) => (
              <li key={`${item.kind}-${item.id}`}>
                <InboxLead item={item} unread={wasUnread(item)} now={now} />
                <PostCard post={item.post} now={now} />
              </li>
            ))}
          </ul>
          {isFetchNextPageError ? <ActionError error={error} className="mt-4" /> : null}
          {hasNextPage ? (
            <div className="mt-6 flex justify-center">
              <Button icon={<ChevronDown />} loading={isFetchingNextPage} onClick={() => void fetchNextPage()}>
                {t('peerbonus.common.loadMore')}
              </Button>
            </div>
          ) : data.pages.length > 1 ? (
            <p className="mt-6 text-center text-xs text-stone-500">{t('peerbonus.inbox.end')}</p>
          ) : null}
        </>
      )}
    </div>
  )
}

/** "鈴木 一郎 さんが拍手しました · 3 分前", with a dot while it is unread. */
function InboxLead({ item, unread, now }: { item: InboxItem; unread: boolean; now: number }) {
  const { t, locale } = useText()
  const name = item.actor.displayName
  return (
    <p className="mb-1.5 flex min-w-0 items-center gap-2 px-1 text-sm">
      <span aria-hidden className={cn('size-2 shrink-0 rounded-full', unread ? 'bg-accent' : 'bg-transparent')} />
      {unread ? <span className="sr-only">{t('peerbonus.inbox.unread')}: </span> : null}
      <Avatar name={name} avatarUrl={item.actor.avatarUrl} size="sm" />
      <span className={cn('min-w-0 truncate', unread ? 'font-semibold text-stone-900' : 'text-stone-600')}>
        {item.kind === 'clap' ? t('peerbonus.inbox.leadClap', { name }) : t('peerbonus.inbox.leadPost', { name })}
      </span>
      <time dateTime={item.createdAt} title={formatDateTime(item.createdAt, locale)} className="shrink-0 text-xs text-stone-500">
        {formatRelativeTime(item.createdAt, now, locale)}
      </time>
    </p>
  )
}
