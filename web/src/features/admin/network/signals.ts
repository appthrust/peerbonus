// The four signal groups a manager acts on (design "v3: network map" → Screen),
// shared by the cards, the graph's badges and the legend so one colour means
// one thing everywhere. Order is the badge priority: a node shows only the
// first group it belongs to.
import type { NetworkNode } from '../../../api/peerbonus'
import type { MessageId } from '../../../copy'

export type SignalGroup = 'concentrated' | 'declining' | 'few' | 'unclapped'

/** `fill`: the SVG badge; `dot`: the same colour on HTML (cards, legend, tooltip). */
export const SIGNAL_GROUPS: readonly { key: SignalGroup; label: MessageId; fill: string; dot: string }[] = [
  { key: 'concentrated', label: 'peerbonus.network.signal.concentrated', fill: 'fill-amber-500', dot: 'bg-amber-500' },
  { key: 'declining', label: 'peerbonus.network.signal.declining', fill: 'fill-rose-500', dot: 'bg-rose-500' },
  { key: 'few', label: 'peerbonus.network.signal.few', fill: 'fill-sky-500', dot: 'bg-sky-500' },
  { key: 'unclapped', label: 'peerbonus.network.signal.unclapped', fill: 'fill-violet-500', dot: 'bg-violet-500' },
]

/** Whether `node` belongs to `group`: つながりが少ない gathers isolated and few-connection people. */
export function hasSignal(node: Pick<NetworkNode, 'signals'>, group: SignalGroup): boolean {
  if (group === 'few') return node.signals.includes('isolated') || node.signals.includes('fewConnections')
  return node.signals.includes(group)
}

/** The node's one badge (the first group by priority), or undefined. */
export function badgeOf(node: Pick<NetworkNode, 'signals'>): (typeof SIGNAL_GROUPS)[number] | undefined {
  return SIGNAL_GROUPS.find((group) => hasSignal(node, group.key))
}
