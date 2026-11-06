// Network map wording (`/admin/network`, design "v3: network map"): the
// summary, the signal cards, the graph's legend and tooltip. Numbers in the
// rules come from the response's `thresholds`, never from this file.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const networkMessages = defineDashboardMessages({
  'peerbonus.admin.tab.network': { defaultMessage: 'Network', translations: { ja: 'つながり' } },
  'peerbonus.network.lead': {
    defaultMessage: 'Who exchanged posts and claps with whom in {month}, and who may need a word.',
    translations: { ja: '{month}に投稿と拍手でつながった人と、声をかけたい人です。' },
  },
  'peerbonus.network.group': { defaultMessage: 'Group', translations: { ja: 'グループ' } },
  'peerbonus.network.allGroups': { defaultMessage: 'All groups', translations: { ja: 'すべて' } },
  'peerbonus.network.directoryDownBody': {
    defaultMessage: 'Only members with activity are shown and groups are unavailable; people with no activity appear again once Directory is back.',
    translations: { ja: '動きのあったメンバーだけを表示しており、グループでは絞り込めません。Directory が戻ると、動きのない人も再び表示されます。' },
  },

  // Summary
  'peerbonus.network.people': { defaultMessage: 'People shown', translations: { ja: '対象' } },
  'peerbonus.network.connected': { defaultMessage: 'With connections', translations: { ja: 'つながりあり' } },
  'peerbonus.network.isolated': { defaultMessage: 'No connections', translations: { ja: 'つながりなし' } },

  // Signal cards
  'peerbonus.network.signals': { defaultMessage: 'People to look after', translations: { ja: '気にかけたい人' } },
  'peerbonus.network.signal.concentrated': { defaultMessage: 'Concentrated', translations: { ja: '集中' } },
  'peerbonus.network.signal.concentratedLead': {
    defaultMessage: 'Received {share}% or more of the month’s posts (at least {posts}).',
    translations: { ja: '今月の投稿の {share}% 以上（{posts} 件以上）を受け取った人' },
  },
  'peerbonus.network.signal.concentratedValue': { defaultMessage: '{posts} posts · {share}%', translations: { ja: '{posts} 件・{share}%' } },
  'peerbonus.network.signal.declining': { defaultMessage: 'Sharp drop', translations: { ja: '利用の急減' } },
  'peerbonus.network.signal.decliningLead': {
    defaultMessage: 'Posts and claps in the last 14 days fell to {ratio}% or less of the 14 days before (which had {minimum} or more).',
    translations: { ja: '直近 14 日の投稿と拍手が、その前の 14 日（{minimum} 回以上）の {ratio}% 以下に減った人' },
  },
  'peerbonus.network.signal.decliningValue': { defaultMessage: '{previous} → {recent} (−{drop}%)', translations: { ja: '{previous} → {recent} 回（−{drop}%）' } },
  'peerbonus.network.signal.few': { defaultMessage: 'Few connections', translations: { ja: 'つながりが少ない' } },
  'peerbonus.network.signal.fewLead': {
    defaultMessage: 'Connected with {few} people or fewer this month, or with nobody.',
    translations: { ja: '今月つながった相手が {few} 人以下、または誰ともつながっていない人' },
  },
  'peerbonus.network.signal.isolatedValue': { defaultMessage: 'Nobody', translations: { ja: 'つながりなし' } },
  'peerbonus.network.signal.unclapped': { defaultMessage: 'Unclapped', translations: { ja: '未拍手' } },
  'peerbonus.network.signal.unclappedLead': {
    defaultMessage: 'Received a post this month that nobody has clapped yet.',
    translations: { ja: '今月受け取った投稿に、まだ誰も拍手していない人' },
  },
  'peerbonus.network.signal.hub': { defaultMessage: 'Hub', translations: { ja: 'ハブ' } },
  'peerbonus.network.more': { defaultMessage: '+{count} more', translations: { ja: '+{count} 人' } },
  'peerbonus.network.none': { defaultMessage: 'Nobody this month.', translations: { ja: 'この月に該当する人はいません。' } },

  // Graph
  'peerbonus.network.graph': { defaultMessage: 'Connection map', translations: { ja: 'つながりマップ' } },
  'peerbonus.network.graphLabel': { defaultMessage: 'Connections between {count} people', translations: { ja: '{count} 人のつながり' } },
  'peerbonus.network.graphHint': {
    defaultMessage: 'Drag or scroll to move around; select a person to open their page.',
    translations: { ja: 'ドラッグやスクロールで動かせます。人を選ぶとその人のページを開きます。' },
  },
  'peerbonus.network.noPeople': { defaultMessage: 'Nobody in this group.', translations: { ja: 'このグループには誰もいません。' } },
  'peerbonus.network.legend': { defaultMessage: 'Legend', translations: { ja: '凡例' } },
  'peerbonus.network.legend.size': { defaultMessage: 'Size: connections', translations: { ja: '大きさ：つながりの数' } },
  'peerbonus.network.legend.hub': { defaultMessage: 'Hub ({many}+)', translations: { ja: 'ハブ（{many} 人以上）' } },
  'peerbonus.network.legend.thisWeek': { defaultMessage: 'This week', translations: { ja: '今週のやりとり' } },
  'peerbonus.network.legend.earlier': { defaultMessage: 'Earlier', translations: { ja: 'それ以前' } },
  'peerbonus.network.legend.exchange': { defaultMessage: 'Exchanges (thicker: more)', translations: { ja: 'やりとり（太いほど多い）' } },
  'peerbonus.network.nodeLabel': { defaultMessage: '{name}, {count} connections', translations: { ja: '{name}（つながり {count} 人）' } },
  'peerbonus.network.tip.connections': { defaultMessage: '{count} connections', translations: { ja: 'つながり {count} 人' } },
  'peerbonus.network.tip.received': { defaultMessage: 'Received {posts} posts · {points} pt', translations: { ja: 'もらった {posts} 件・{points} pt' } },
  'peerbonus.network.tip.sent': { defaultMessage: 'Sent {posts} posts', translations: { ja: '贈った {posts} 件' } },
  'peerbonus.network.tip.claps': { defaultMessage: 'Claps given {given} · received {received}', translations: { ja: '拍手した {given} 回・もらった {received} 回' } },

  // Empty month
  'peerbonus.network.empty': { defaultMessage: 'No posts or claps in {month} yet', translations: { ja: '{month}の投稿や拍手はまだありません' } },
  'peerbonus.network.emptyBody': {
    defaultMessage: 'Once people send posts and clap, who is connected to whom shows up here.',
    translations: { ja: '投稿や拍手が届くと、誰と誰がつながっているかがここに描かれます。' },
  },
})
