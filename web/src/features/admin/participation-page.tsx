import { useSuspenseQuery } from '@tanstack/react-query'
import { Users } from 'lucide-react'
import type { Participant, Participation } from '../../api/peerbonus'
import type { MessageId } from '../../copy'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { Avatar, cn, EmptyState, HrefLink, LinkButton, Notice, PageHeader, StatCard, useDocumentTitle } from '../../ui'
import { PendingMark } from '../members/profile-parts'
import { formatCount, formatMonth } from './format'
import { MonthPicker, monthOptions } from './month-picker'
import { adminMembersQuery, statementsQuery } from './queries'

type Count = 'receivedPoints' | 'receivedPosts' | 'sentPosts' | 'sentPoints' | 'clapsGiven'

const COUNTS: readonly { key: Count; label: MessageId }[] = [
  { key: 'receivedPoints', label: 'peerbonus.admin.col.receivedPoints' },
  { key: 'receivedPosts', label: 'peerbonus.member.receivedPosts' },
  { key: 'sentPosts', label: 'peerbonus.member.sentPosts' },
  { key: 'sentPoints', label: 'peerbonus.admin.participation.col.sentPoints' },
  { key: 'clapsGiven', label: 'peerbonus.admin.participation.col.clapsGiven' },
]

/** Gave, received or clapped in the month (a reversal alone also counts: points moved). */
export function isActive(row: Participant): boolean {
  return row.sentPosts > 0 || row.receivedPosts > 0 || row.clapsGiven > 0 || row.receivedPoints !== 0
}

/** Avatar, name (a profile link only once they are a member), the quiet pending mark and the email. */
function Person({ row }: { row: Participant }) {
  const { t } = useText()
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <Avatar name={row.displayName} avatarUrl={row.avatarUrl} size="md" />
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-1">
          {row.member ? (
            <HrefLink href={paths.member(row.member.id)} className="truncate font-medium text-stone-900 hover:underline">
              {row.displayName}
            </HrefLink>
          ) : (
            <span className="truncate font-medium text-stone-700">{row.displayName}</span>
          )}
          {row.pending ? <PendingMark hint={t('peerbonus.admin.pendingHint')} /> : null}
        </div>
        <div className="truncate text-xs text-stone-500">{row.email}</div>
      </div>
    </div>
  )
}

/** Why a row has no numbers: never part of Peer Bonus yet, or simply quiet this month. */
function InactiveNote({ row }: { row: Participant }) {
  const { t } = useText()
  return <span className="text-xs text-stone-400">{t(row.member ? 'peerbonus.admin.participation.quiet' : 'peerbonus.admin.participation.notYet')}</span>
}

function ParticipationTable({ data }: { data: Participation }) {
  const { t, locale } = useText()
  return (
    <div className="hidden overflow-hidden rounded-lg border border-line bg-white md:block">
      <table className="w-full text-sm">
        <caption className="sr-only">
          {t('peerbonus.admin.tab.members')} – {formatMonth(data.month, locale)}
        </caption>
        <thead className="border-b border-line bg-stone-50 text-left text-xs font-medium text-stone-500">
          <tr>
            <th scope="col" className="px-4 py-2.5">
              {t('peerbonus.admin.col.name')}
            </th>
            {COUNTS.map((column) => (
              <th key={column.key} scope="col" className="px-3 py-2.5 text-right whitespace-nowrap" aria-sort={column.key === 'receivedPoints' ? 'descending' : undefined}>
                {t(column.label)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-stone-100">
          {data.items.map((row) => {
            const active = isActive(row)
            return (
              <tr key={row.personId} className={active ? undefined : 'bg-stone-50/60'}>
                <th scope="row" className="max-w-72 px-4 py-2.5 text-left font-normal">
                  <Person row={row} />
                </th>
                {active ? (
                  COUNTS.map((column) => (
                    <td key={column.key} className={cn('px-3 py-2.5 text-right tabular-nums', column.key === 'receivedPoints' ? 'font-semibold text-stone-900' : 'text-stone-700')}>
                      {formatCount(row[column.key], locale)}
                    </td>
                  ))
                ) : (
                  <td colSpan={COUNTS.length} className="px-3 py-2.5 text-right">
                    <InactiveNote row={row} />
                  </td>
                )}
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

/** Below `md` every person is a card row: name on top, then received, sent and claps under it (principle 9). */
function ParticipationCards({ data }: { data: Participation }) {
  const { t, locale } = useText()
  return (
    <ul aria-label={t('peerbonus.admin.tab.members')} className="divide-y divide-stone-100 rounded-lg border border-line bg-white md:hidden">
      {data.items.map((row) => {
        const active = isActive(row)
        return (
          <li key={row.personId} className={cn('px-4 py-3', active ? undefined : 'bg-stone-50/60')}>
            <Person row={row} />
            {active ? (
              <dl className="mt-2.5 grid grid-cols-3 gap-2 pl-10.5 text-xs">
                <div className="min-w-0">
                  <dt className="text-stone-500">{t('peerbonus.admin.participation.recipients')}</dt>
                  <dd className="tabular-nums">
                    <span className="block font-semibold text-stone-900">{t('peerbonus.common.points', { points: formatCount(row.receivedPoints, locale) })}</span>
                    <span className="block text-stone-500">{t('peerbonus.member.postCount', { count: row.receivedPosts })}</span>
                  </dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-stone-500">{t('peerbonus.admin.participation.senders')}</dt>
                  <dd className="tabular-nums">
                    <span className="block text-stone-800">{t('peerbonus.common.points', { points: formatCount(row.sentPoints, locale) })}</span>
                    <span className="block text-stone-500">{t('peerbonus.member.postCount', { count: row.sentPosts })}</span>
                  </dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-stone-500">{t('peerbonus.admin.participation.col.clapsGiven')}</dt>
                  <dd className="text-stone-800 tabular-nums">{formatCount(row.clapsGiven, locale)}</dd>
                </div>
              </dl>
            ) : (
              <p className="mt-1 pl-10.5">
                <InactiveNote row={row} />
              </p>
            )}
          </li>
        )
      })}
    </ul>
  )
}

/**
 * `/admin/members?month=`: everyone in Directory with what they did that month (principle 10:
 * participation first). People who did nothing are listed too, quietly; while Directory is
 * unreachable only members with activity are known, and a notice says so.
 */
export function ParticipationPage({ month, currentMonth, onMonthChange }: { month: string; currentMonth: string; onMonthChange: (month: string) => void }) {
  const { t, locale } = useText()
  const { data } = useSuspenseQuery(adminMembersQuery(month))
  const { data: statements } = useSuspenseQuery(statementsQuery)
  useDocumentTitle(t('peerbonus.admin.tab.members'))
  const { summary } = data
  const rate = summary.people > 0 ? Math.round((summary.active / summary.people) * 100) : 0

  return (
    <div>
      <PageHeader
        title={t('peerbonus.admin.tab.members')}
        lead={t('peerbonus.admin.participation.lead', { month: formatMonth(month, locale) })}
        actions={<MonthPicker month={month} months={monthOptions(month, currentMonth, statements)} onChange={onMonthChange} />}
      />

      {data.directoryAvailable ? null : (
        <Notice tone="warning" title={t('peerbonus.admin.participation.directoryDown')} className="mb-4">
          {t('peerbonus.admin.participation.directoryDownBody')}
        </Notice>
      )}

      <dl className="mb-6 grid grid-cols-2 gap-2 sm:gap-3 lg:grid-cols-4">
        <StatCard label={t('peerbonus.admin.participation.people')} value={formatCount(summary.people, locale)} unit={t('peerbonus.admin.participation.peopleUnit')} />
        <StatCard label={t('peerbonus.admin.participation.senders')} value={formatCount(summary.senders, locale)} unit={t('peerbonus.admin.participation.peopleUnit')} />
        <StatCard label={t('peerbonus.admin.participation.recipients')} value={formatCount(summary.recipients, locale)} unit={t('peerbonus.admin.participation.peopleUnit')} />
        <StatCard
          label={t('peerbonus.admin.participation.rate')}
          value={rate}
          unit="%"
          note={t('peerbonus.admin.participation.rateNote', { active: summary.active, inactive: summary.people - summary.active })}
        />
      </dl>

      {data.items.length === 0 ? (
        <EmptyState
          icon={<Users />}
          title={t('peerbonus.admin.participation.empty')}
          body={t('peerbonus.admin.participation.emptyBody')}
          action={<LinkButton href={paths.home}>{t('peerbonus.member.openTimeline')}</LinkButton>}
        />
      ) : (
        <>
          <ParticipationTable data={data} />
          <ParticipationCards data={data} />
        </>
      )}
    </div>
  )
}
