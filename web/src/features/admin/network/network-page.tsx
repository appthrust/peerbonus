import { useSuspenseQuery } from '@tanstack/react-query'
import { Gift, Network as NetworkIcon } from 'lucide-react'
import { useId } from 'react'
import type { NetworkGroup } from '../../../api/peerbonus'
import { networkQuery } from '../../../api/queries'
import { useText } from '../../../i18n'
import { paths } from '../../../routes/paths'
import { useSession } from '../../../shell/use-session'
import { cn, controlClasses, EmptyState, LinkButton, Notice, PageHeader, Section, StatCard, useDocumentTitle } from '../../../ui'
import { formatCount, formatMonth } from '../format'
import { MonthPicker, monthOptions } from '../month-picker'
import { statementsQuery } from '../queries'
import { NetworkGraph } from './network-graph'
import { SignalCards } from './signal-cards'

/** `?month=&group=` of the page: this month and everyone are the defaults and stay out of the URL. */
export type NetworkSearch = { month?: string; group?: string }

export function networkSearch(month: string, group: string, currentMonth: string): NetworkSearch {
  return { ...(month === currentMonth ? {} : { month }), ...(group ? { group } : {}) }
}

/** すべて, then every Directory group by name; the choice is the page's `?group=`. */
function GroupPicker({ group, groups, onChange }: { group: string; groups: readonly NetworkGroup[]; onChange: (group: string) => void }) {
  const { t } = useText()
  const id = useId()
  return (
    <div className="flex items-center gap-2">
      <label htmlFor={id} className="text-sm font-medium text-stone-700">
        {t('peerbonus.network.group')}
      </label>
      <select id={id} value={group} onChange={(event) => onChange(event.target.value)} className={cn(controlClasses, 'h-10 max-w-48 pr-8 md:h-9')}>
        <option value="">{t('peerbonus.network.allGroups')}</option>
        {groups.map((option) => (
          <option key={option.id} value={option.id}>
            {option.name}
          </option>
        ))}
      </select>
    </div>
  )
}

/**
 * `/admin/network?month=&group=`: who exchanged posts and claps with whom in the month (design "v3: network
 * map"). Signal cards first (above the graph on phones), then the graph; numbers in the rules come from the
 * response's thresholds.
 */
export function NetworkPage({
  month,
  group,
  currentMonth,
  onChange,
}: {
  month: string
  group: string
  currentMonth: string
  onChange: (next: { month: string; group: string }) => void
}) {
  const { t, locale } = useText()
  const { me } = useSession()
  const { data } = useSuspenseQuery(networkQuery(month, group))
  const { data: statements } = useSuspenseQuery(statementsQuery)
  useDocumentTitle(t('peerbonus.admin.tab.network'))
  const { summary } = data
  const peopleUnit = t('peerbonus.admin.participation.peopleUnit')
  const quietMonth = summary.posts === 0 && summary.claps === 0

  return (
    <div>
      <PageHeader
        title={t('peerbonus.admin.tab.network')}
        lead={t('peerbonus.network.lead', { month: formatMonth(month, locale) })}
        actions={
          <>
            <MonthPicker month={month} months={monthOptions(month, currentMonth, statements)} onChange={(next) => onChange({ month: next, group })} />
            {data.groups.length > 0 ? <GroupPicker group={data.group?.id ?? ''} groups={data.groups} onChange={(next) => onChange({ month, group: next })} /> : null}
          </>
        }
      />

      {data.directoryAvailable ? null : (
        <Notice tone="warning" title={t('peerbonus.admin.participation.directoryDown')} className="mb-4">
          {t('peerbonus.network.directoryDownBody')}
        </Notice>
      )}

      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-3 lg:grid-cols-5">
        <StatCard label={t('peerbonus.network.people')} value={formatCount(summary.people, locale)} unit={peopleUnit} />
        <StatCard label={t('peerbonus.network.connected')} value={formatCount(summary.connected, locale)} unit={peopleUnit} />
        <StatCard label={t('peerbonus.network.isolated')} value={formatCount(summary.isolated, locale)} unit={peopleUnit} />
        <StatCard label={t('peerbonus.admin.insights.posts')} value={formatCount(summary.posts, locale)} unit={t('peerbonus.member.postsUnit')} />
        <StatCard label={t('peerbonus.admin.participation.col.clapsGiven')} value={formatCount(summary.claps, locale)} unit={t('peerbonus.admin.insights.clapsUnit')} />
      </dl>

      {quietMonth ? (
        <div className="mt-8">
          <EmptyState
            icon={<NetworkIcon />}
            title={t('peerbonus.network.empty', { month: formatMonth(month, locale) })}
            body={t('peerbonus.network.emptyBody')}
            action={
              <LinkButton tone="primary" icon={<Gift />} href={paths.home}>
                {t('peerbonus.admin.insights.emptyAction')}
              </LinkButton>
            }
          />
        </div>
      ) : (
        <div className="mt-8 grid gap-8 lg:grid-cols-[minmax(0,18rem)_minmax(0,1fr)] lg:gap-6">
          <div className="min-w-0">
            <Section title={t('peerbonus.network.signals')}>
              <SignalCards data={data} />
            </Section>
          </div>
          <div className="min-w-0">
            <Section title={t('peerbonus.network.graph')} lead={t('peerbonus.network.graphHint')}>
              {data.nodes.length === 0 ? (
                <p className="rounded-lg border border-dashed border-stone-300 bg-white px-4 py-4 text-sm text-stone-500">{t('peerbonus.network.noPeople')}</p>
              ) : (
                <NetworkGraph data={data} viewer={me.member.personId} />
              )}
            </Section>
          </div>
        </div>
      )}
    </div>
  )
}
