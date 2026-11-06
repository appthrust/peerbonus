// Pieces of the profile story shared by the member page and admin insights:
// partner rows, the value distribution, the 6-month chart and the quiet
// "never signed in" mark (principle 7: an icon with a tooltip, not a badge).
import { Clock } from 'lucide-react'
import type { ReactNode } from 'react'
import type { MonthPoint, Partner, Value, ValueCount } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { Avatar, Bar, cn, HrefLink, hueDot, Tooltip, valueHue } from '../../ui'
import { formatCount, formatMonth, formatMonthShort } from '../admin/format'

/** A clock after the name of someone who has never signed in; the tooltip and the sr-only text say what it means. */
export function PendingMark({ hint }: { hint?: string }) {
  const { t } = useText()
  const text = hint ?? t('peerbonus.member.pendingHint')
  return (
    <Tooltip content={text}>
      <span tabIndex={0} className="relative z-[1] inline-flex size-5 shrink-0 items-center justify-center rounded-full text-stone-400 hover:text-stone-600 focus-visible:outline-2 focus-visible:outline-accent">
        <Clock aria-hidden className="size-3.5" />
        <span className="sr-only">{t('peerbonus.common.pending')}</span>
      </span>
    </Tooltip>
  )
}

/** The label of a value key; a key the Program no longer defines reads "#key" (principle 4). */
export function valueLabel(values: readonly Value[], key: string): string {
  return values.find((value) => value.key === key)?.label ?? `#${key}`
}

/**
 * Ranked people with their points and posts (top partners, top recipients / senders). `empty`
 * replaces the list when there is nobody yet.
 */
export function PartnerList({ partners, empty, label }: { partners: Partner[]; empty: ReactNode; label: string }) {
  const { t, locale } = useText()
  if (partners.length === 0) return <>{empty}</>
  const max = Math.max(...partners.map((partner) => partner.points), 1)
  return (
    <ol aria-label={label} className="divide-y divide-stone-100 rounded-lg border border-line bg-white">
      {partners.map((partner, index) => (
        <li key={partner.member.id} className="flex items-center gap-3 px-3 py-2.5 sm:px-4">
          <span className="w-4 shrink-0 text-right text-xs font-medium text-stone-400 tabular-nums">{index + 1}</span>
          <Avatar name={partner.member.displayName} avatarUrl={partner.member.avatarUrl} size="md" />
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-1">
              <HrefLink href={paths.member(partner.member.id)} className="truncate text-sm font-medium text-stone-900 hover:underline">
                {partner.member.displayName}
              </HrefLink>
              {partner.member.pending ? <PendingMark /> : null}
            </div>
            <Bar value={partner.points} max={max} className="mt-1.5 h-1.5" />
          </div>
          <div className="shrink-0 text-right">
            <div className="text-sm font-semibold text-stone-900 tabular-nums">{t('peerbonus.common.points', { points: formatCount(partner.points, locale) })}</div>
            <div className="text-xs text-stone-500 tabular-nums">{t('peerbonus.member.postCount', { count: partner.posts })}</div>
          </div>
        </li>
      ))}
    </ol>
  )
}

/**
 * Posts per value as horizontal bars in the value's colour, most first; each row opens the
 * timeline narrowed to that value.
 */
export function ValueBars({ items, values, empty, label }: { items: ValueCount[]; values: readonly Value[]; empty: ReactNode; label: string }) {
  const { t, locale } = useText()
  if (items.length === 0) return <>{empty}</>
  const max = Math.max(...items.map((item) => item.posts), 1)
  return (
    <ul aria-label={label} className="space-y-1 rounded-lg border border-line bg-white px-2 py-2 sm:px-3">
      {items.map((item) => {
        const name = valueLabel(values, item.key)
        return (
          <li key={item.key}>
            <HrefLink
              href={paths.timeline({ value: item.key })}
              aria-label={t('peerbonus.member.valueOpen', { value: name, count: item.posts })}
              className="grid min-h-10 grid-cols-[minmax(0,7rem)_minmax(0,1fr)_auto] items-center gap-3 rounded-md px-2 py-1 hover:bg-stone-50 sm:grid-cols-[minmax(0,10rem)_minmax(0,1fr)_auto]"
            >
              <span className="flex min-w-0 items-center gap-2 text-sm text-stone-800">
                <span aria-hidden className={cn('size-2 shrink-0 rounded-full', hueDot(valueHue(values, item.key)))} />
                <span className="truncate">{name}</span>
              </span>
              <Bar value={item.posts} max={max} />
              <span className="text-right text-xs text-stone-500 tabular-nums">
                <span className="font-semibold text-stone-800">{t('peerbonus.member.postCount', { count: item.posts })}</span>
                <span className="hidden sm:inline"> · {t('peerbonus.common.points', { points: formatCount(item.points, locale) })}</span>
              </span>
            </HrefLink>
          </li>
        )
      })}
    </ul>
  )
}

/** Received points per month (oldest first) as columns; the current month is the accent one. */
export function MonthChart({ months }: { months: MonthPoint[] }) {
  const { t, locale } = useText()
  const max = Math.max(...months.map((month) => month.receivedPoints), 1)
  const last = months.length - 1
  return (
    <ol className="grid grid-flow-col gap-2 rounded-lg border border-line bg-white px-3 pt-4 pb-3 sm:gap-4 sm:px-5" style={{ gridAutoColumns: 'minmax(0, 1fr)' }}>
      {months.map((month, index) => (
        <li key={month.key} className="flex min-w-0 flex-col items-center gap-1.5">
          <span className="sr-only">
            {t('peerbonus.member.monthBar', { month: formatMonth(month.key, locale), points: month.receivedPoints, sent: month.sentPosts })}
          </span>
          <span aria-hidden className="text-xs font-semibold text-stone-700 tabular-nums">
            {formatCount(month.receivedPoints, locale)}
          </span>
          <span aria-hidden className="block h-24 w-full max-w-10 sm:h-28">
            <Bar value={month.receivedPoints} max={max} orientation="vertical" tone={index === last ? 'accent' : 'muted'} />
          </span>
          <span aria-hidden className={cn('text-xs tabular-nums', index === last ? 'font-semibold text-stone-900' : 'text-stone-500')}>
            {formatMonthShort(month.key, locale)}
          </span>
          <span aria-hidden className="text-[11px] whitespace-nowrap text-stone-400 tabular-nums" title={t('peerbonus.member.monthSentHint')}>
            {t('peerbonus.member.monthSent', { count: month.sentPosts })}
          </span>
        </li>
      ))}
    </ol>
  )
}
