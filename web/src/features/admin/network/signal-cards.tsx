import { useId } from 'react'
import type { Network, NetworkNode } from '../../../api/peerbonus'
import { useText, type Translate } from '../../../i18n'
import { paths } from '../../../routes/paths'
import { Avatar, cn, HrefLink } from '../../../ui'
import { PendingMark } from '../../members/profile-parts'
import { hasSignal, SIGNAL_GROUPS, type SignalGroup } from './signals'

/** People a card names before "+N". */
const CARD_LIMIT = 5

/** "−60%": how far recent activity fell below the previous window. */
const dropPercent = (decline: { recent: number; previous: number }) => (decline.previous > 0 ? Math.round((1 - decline.recent / decline.previous) * 100) : 0)

/** The people of one group, the most pressing first. */
function membersOf(nodes: readonly NetworkNode[], group: SignalGroup): NetworkNode[] {
  const byName = (a: NetworkNode, b: NetworkNode) => a.displayName.localeCompare(b.displayName) || a.personId.localeCompare(b.personId)
  const rows = nodes.filter((node) => hasSignal(node, group))
  switch (group) {
    case 'concentrated':
      return rows.sort((a, b) => b.receivedPosts - a.receivedPosts || byName(a, b))
    case 'declining':
      return rows.sort((a, b) => dropPercent(b.decline ?? { recent: 0, previous: 0 }) - dropPercent(a.decline ?? { recent: 0, previous: 0 }) || byName(a, b))
    case 'few':
      return rows.sort((a, b) => a.connections - b.connections || byName(a, b))
    case 'unclapped':
      return rows.sort((a, b) => b.unclappedPosts - a.unclappedPosts || byName(a, b))
  }
}

/** The echoed number next to a name. */
function valueOf(node: NetworkNode, group: SignalGroup, t: Translate): string {
  switch (group) {
    case 'concentrated':
      return t('peerbonus.network.signal.concentratedValue', { posts: node.receivedPosts, share: Math.round(node.share * 100) })
    case 'declining':
      return node.decline ? t('peerbonus.network.signal.decliningValue', { previous: node.decline.previous, recent: node.decline.recent, drop: dropPercent(node.decline) }) : ''
    case 'few':
      return node.connections === 0 ? t('peerbonus.network.signal.isolatedValue') : t('peerbonus.admin.statement.people', { count: node.connections })
    case 'unclapped':
      return t('peerbonus.member.postCount', { count: node.unclappedPosts })
  }
}

/** The rule in words, with the server's thresholds. */
function leadOf(group: SignalGroup, thresholds: Network['thresholds'], t: Translate): string {
  switch (group) {
    case 'concentrated':
      return t('peerbonus.network.signal.concentratedLead', { share: Math.round(thresholds.concentrationShare * 100), posts: thresholds.concentrationPosts })
    case 'declining':
      return t('peerbonus.network.signal.decliningLead', { ratio: Math.round(thresholds.declineRatio * 100), minimum: thresholds.declineMinimum })
    case 'few':
      return t('peerbonus.network.signal.fewLead', { few: thresholds.fewConnections })
    case 'unclapped':
      return t('peerbonus.network.signal.unclappedLead')
  }
}

function SignalCard({ group, data }: { group: (typeof SIGNAL_GROUPS)[number]; data: Network }) {
  const { t } = useText()
  const headingId = useId()
  const people = membersOf(data.nodes, group.key)
  const shown = people.slice(0, CARD_LIMIT)
  return (
    <section aria-labelledby={headingId} className="min-w-0 rounded-lg border border-line bg-white px-4 py-3">
      <div className="flex items-center gap-2">
        <span aria-hidden className={cn('size-2.5 shrink-0 rounded-full', group.dot)} />
        <h3 id={headingId} className="min-w-0 flex-1 truncate text-sm font-semibold text-stone-900">
          {t(group.label)}
        </h3>
        <span className="text-sm font-semibold text-stone-700 tabular-nums">{people.length}</span>
      </div>
      <p className="mt-1 text-xs text-stone-500">{leadOf(group.key, data.thresholds, t)}</p>
      {shown.length === 0 ? (
        <p className="mt-2 text-xs text-stone-400">{t('peerbonus.network.none')}</p>
      ) : (
        <ul className="mt-2 divide-y divide-stone-100">
          {shown.map((node) => (
            <li key={node.personId} className="flex min-h-10 items-center gap-2 py-1.5">
              <Avatar name={node.displayName} avatarUrl={node.avatarUrl} size="sm" />
              <span className="flex min-w-0 flex-1 items-center gap-1">
                {node.memberId ? (
                  <HrefLink href={paths.member(node.memberId)} className="truncate text-sm font-medium text-stone-900 hover:underline">
                    {node.displayName}
                  </HrefLink>
                ) : (
                  <span className="truncate text-sm font-medium text-stone-700">{node.displayName}</span>
                )}
                {node.pending ? <PendingMark hint={t('peerbonus.admin.pendingHint')} /> : null}
              </span>
              <span className="shrink-0 text-xs text-stone-600 tabular-nums">{valueOf(node, group.key, t)}</span>
            </li>
          ))}
        </ul>
      )}
      {people.length > shown.length ? <p className="mt-1 text-xs font-medium text-stone-500">{t('peerbonus.network.more', { count: people.length - shown.length })}</p> : null}
    </section>
  )
}

/** 集中 / 利用の急減 / つながりが少ない / 未拍手: up to five people each, with the number behind the signal. */
export function SignalCards({ data }: { data: Network }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
      {SIGNAL_GROUPS.map((group) => (
        <SignalCard key={group.key} group={group} data={data} />
      ))}
    </div>
  )
}
