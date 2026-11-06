import { useQueryClient } from '@tanstack/react-query'
import { Gift, HandCoins, Hourglass, SearchX } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { Me, Post } from '../../api/peerbonus'
import { prependPost } from '../../api/queries'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { useSession } from '../../shell/use-session'
import { Button, CLOCK_TICK_MS, EmptyState, LinkButton, useDocumentTitle, useNow } from '../../ui'
import { ComposeLauncher } from '../compose/launcher'
import { FilterBar, type TimelineFilters } from './filter-bar'
import { PostList } from './post-list'

/** The nudge shows from this long before the week's budget expires. */
const REMINDER_WINDOW_MS = 48 * 60 * 60 * 1000
/** How long a just-sent post keeps its highlight (the CSS glow is shorter). */
const HIGHLIGHT_MS = 3_000

/** Whether to nudge: at least half the week's budget left and the week ends within two days. */
export function shouldRemind(stats: Me['stats']['period'], now: number): boolean {
  const left = Date.parse(stats.endsAt) - now
  return stats.remaining > 0 && stats.remaining * 2 >= stats.allowance && left > 0 && left <= REMINDER_WINDOW_MS
}

/**
 * `/`: the reminder (when the week's budget is about to expire unused), the
 * filter bar, the compose launcher, then the feed. `to` (`?to=<personId>`)
 * opens the launcher with that recipient and `compose` (`?compose=1`) opens
 * it empty; the page then drops both from the URL. A sent post goes on top
 * of the feed at once, highlighted.
 */
export function TimelinePage({
  filters,
  to,
  compose = false,
  onFiltersChange,
  onPreselected,
  onComposeHandled,
}: {
  filters: TimelineFilters
  to?: string
  compose?: boolean
  onFiltersChange: (next: TimelineFilters, options?: { replace?: boolean }) => void
  onPreselected?: () => void
  onComposeHandled?: () => void
}) {
  const { t, locale } = useText()
  const { me } = useSession()
  const client = useQueryClient()
  useDocumentTitle(t('peerbonus.nav.home'))
  // One clock for the page, moving every minute: the reminder, the month list and every card's relative time.
  const now = useNow(CLOCK_TICK_MS)
  const [launcherOpen, setLauncherOpen] = useState(compose || Boolean(to))
  const [highlightedId, setHighlightedId] = useState<string | null>(null)
  const filtered = Boolean(filters.value || filters.month || filters.q || filters.mine)

  // `?compose=1` arriving on an open page opens the launcher in the same render; the URL then drops it.
  const [composeSeen, setComposeSeen] = useState(compose)
  if (compose !== composeSeen) {
    setComposeSeen(compose)
    if (compose) setLauncherOpen(true)
  }
  useEffect(() => {
    if (compose) onComposeHandled?.()
  }, [compose, onComposeHandled])

  useEffect(() => {
    if (!highlightedId) return
    const timer = setTimeout(() => setHighlightedId(null), HIGHLIGHT_MS)
    return () => clearTimeout(timer)
  }, [highlightedId])

  const onSent = (post: Post) => {
    prependPost(client, post)
    setHighlightedId(post.id)
  }

  const period = me.stats.period
  const expires = new Intl.DateTimeFormat(locale, {
    month: 'numeric',
    day: 'numeric',
    weekday: 'short',
    hour: 'numeric',
    minute: '2-digit',
    timeZone: me.program.timeZone,
  }).format(new Date(period.endsAt))

  return (
    <div className="mx-auto max-w-2xl">
      <h1 className="sr-only">{t('peerbonus.nav.home')}</h1>
      {shouldRemind(period, now) ? (
        <div role="status" className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border border-accent-line bg-accent-soft px-4 py-3">
          <Hourglass aria-hidden className="size-5 shrink-0 text-accent" />
          <p className="min-w-0 flex-1 text-sm font-medium text-accent-strong">{t('peerbonus.timeline.reminder', { points: period.remaining, time: expires })}</p>
          <Button tone="primary" size="sm" icon={<Gift />} onClick={() => setLauncherOpen(true)}>
            {t('peerbonus.timeline.reminderAction')}
          </Button>
        </div>
      ) : null}
      <FilterBar filters={filters} now={now} onChange={onFiltersChange} />
      <div className="mt-4">
        <ComposeLauncher to={to} onPreselected={onPreselected} onSent={onSent} open={launcherOpen} onOpenChange={setLauncherOpen} />
      </div>
      <section aria-label={t('peerbonus.timeline.posts')} className="mt-4">
        <PostList
          feed={{ kind: 'all', ...filters }}
          highlightedId={highlightedId}
          now={now}
          empty={
            filtered ? (
              <EmptyState
                icon={<SearchX />}
                title={t('peerbonus.timeline.emptyFiltered.title')}
                body={t('peerbonus.timeline.emptyFiltered.body')}
                action={<LinkButton href={paths.timeline()}>{t('peerbonus.timeline.loosen')}</LinkButton>}
              />
            ) : (
              <EmptyState
                icon={<HandCoins />}
                title={t('peerbonus.timeline.empty.title')}
                body={t('peerbonus.timeline.empty.body')}
                action={
                  <Button tone="primary" icon={<Gift />} onClick={() => setLauncherOpen(true)}>
                    {t('peerbonus.timeline.empty.action')}
                  </Button>
                }
              />
            )
          }
        />
      </section>
    </div>
  )
}
