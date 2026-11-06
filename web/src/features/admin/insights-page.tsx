import { useSuspenseQuery } from '@tanstack/react-query'
import { BarChart3, Gift } from 'lucide-react'
import type { ReactNode } from 'react'
import { insightsQuery } from '../../api/queries'
import type { WeekPoint } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { useSession } from '../../shell/use-session'
import { Bar, EmptyState, LinkButton, PageHeader, Section, StatCard, useDocumentTitle } from '../../ui'
import { PartnerList, ValueBars } from '../members/profile-parts'
import { formatCount, formatMonth } from './format'
import { MonthPicker, monthOptions } from './month-picker'
import { statementsQuery } from './queries'

/** A muted line where a block of an active month has nothing (e.g. no post carried a value). */
function Quiet({ children }: { children: ReactNode }) {
  return <p className="rounded-lg border border-dashed border-stone-300 bg-white px-4 py-4 text-sm text-stone-500">{children}</p>
}

/** Points per ISO week of the month ("10/13–10/19"), in the program time zone; weeks may start in the month before. */
function WeekBars({ weeks, timeZone }: { weeks: WeekPoint[]; timeZone: string }) {
  const { t, locale } = useText()
  const range = new Intl.DateTimeFormat(locale, { month: 'numeric', day: 'numeric', timeZone })
  const max = Math.max(...weeks.map((week) => week.points), 1)
  return (
    <ul className="space-y-1 rounded-lg border border-line bg-white px-2 py-2 sm:px-3">
      {weeks.map((week) => {
        // `endsAt` is the next Monday 00:00; the week's last day is the Sunday before it.
        const label = range.formatRange(new Date(week.startsAt), new Date(Date.parse(week.endsAt) - 1))
        return (
          <li key={week.key} className="grid min-h-10 grid-cols-[minmax(0,6.5rem)_minmax(0,1fr)_auto] items-center gap-3 px-2 py-1 sm:grid-cols-[minmax(0,8rem)_minmax(0,1fr)_auto]">
            <span className="truncate text-sm text-stone-700 tabular-nums" title={week.key}>
              {label}
            </span>
            <Bar value={week.points} max={max} />
            <span className="text-right text-xs text-stone-500 tabular-nums">
              <span className="font-semibold text-stone-800">{t('peerbonus.common.points', { points: formatCount(week.points, locale) })}</span>
              <span className="block sm:inline">
                <span className="hidden sm:inline"> · </span>
                {t('peerbonus.admin.insights.weekDetail', { posts: week.posts, claps: week.claps })}
              </span>
            </span>
          </li>
        )
      })}
    </ul>
  )
}

/**
 * `/admin/insights?month=`: how the month went — totals, which values people recognised,
 * the weekly rhythm, and who received and gave the most.
 */
export function InsightsPage({ month, currentMonth, onMonthChange }: { month: string; currentMonth: string; onMonthChange: (month: string) => void }) {
  const { t, locale } = useText()
  const { me } = useSession()
  const { data } = useSuspenseQuery(insightsQuery(month))
  const { data: statements } = useSuspenseQuery(statementsQuery)
  useDocumentTitle(t('peerbonus.admin.tab.insights'))
  const quietMonth = data.posts === 0 && data.claps === 0
  const peopleUnit = t('peerbonus.admin.participation.peopleUnit')

  return (
    <div>
      <PageHeader
        title={t('peerbonus.admin.tab.insights')}
        lead={t('peerbonus.admin.insights.lead', { month: formatMonth(month, locale) })}
        actions={<MonthPicker month={month} months={monthOptions(month, currentMonth, statements)} onChange={onMonthChange} />}
      />

      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-3 lg:grid-cols-5">
        <StatCard label={t('peerbonus.admin.insights.posts')} value={formatCount(data.posts, locale)} unit={t('peerbonus.member.postsUnit')} />
        <StatCard label={t('peerbonus.admin.col.receivedPoints')} value={formatCount(data.points, locale)} unit={t('peerbonus.common.pointsUnit')} />
        <StatCard label={t('peerbonus.admin.participation.col.clapsGiven')} value={formatCount(data.claps, locale)} unit={t('peerbonus.admin.insights.clapsUnit')} />
        <StatCard label={t('peerbonus.admin.participation.senders')} value={formatCount(data.senders, locale)} unit={peopleUnit} />
        <StatCard label={t('peerbonus.admin.participation.recipients')} value={formatCount(data.recipients, locale)} unit={peopleUnit} />
      </dl>

      {quietMonth ? (
        <div className="mt-8">
          <EmptyState
            icon={<BarChart3 />}
            title={t('peerbonus.admin.insights.empty', { month: formatMonth(month, locale) })}
            body={t('peerbonus.admin.insights.emptyBody')}
            action={
              <LinkButton tone="primary" icon={<Gift />} href={paths.home}>
                {t('peerbonus.admin.insights.emptyAction')}
              </LinkButton>
            }
          />
        </div>
      ) : (
        <>
          <div className="mt-8 grid gap-8 lg:grid-cols-2 lg:gap-6">
            <div className="min-w-0">
              <Section title={t('peerbonus.admin.insights.values')} lead={t('peerbonus.admin.insights.valuesLead')}>
                <ValueBars label={t('peerbonus.admin.insights.values')} items={data.values} values={me.program.values} empty={<Quiet>{t('peerbonus.admin.insights.valuesEmpty')}</Quiet>} />
              </Section>
            </div>
            <div className="min-w-0">
              <Section title={t('peerbonus.admin.insights.weeks')} lead={t('peerbonus.admin.insights.weeksLead')}>
                <WeekBars weeks={data.weeks} timeZone={me.program.timeZone} />
              </Section>
            </div>
          </div>
          <div className="mt-8 grid gap-8 md:grid-cols-2 md:gap-6">
            <div className="min-w-0">
              <Section title={t('peerbonus.admin.insights.topRecipients')}>
                <PartnerList label={t('peerbonus.admin.insights.topRecipients')} partners={data.topRecipients} empty={<Quiet>{t('peerbonus.admin.insights.nobody')}</Quiet>} />
              </Section>
            </div>
            <div className="min-w-0">
              <Section title={t('peerbonus.admin.insights.topSenders')}>
                <PartnerList label={t('peerbonus.admin.insights.topSenders')} partners={data.topSenders} empty={<Quiet>{t('peerbonus.admin.insights.nobody')}</Quiet>} />
              </Section>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
