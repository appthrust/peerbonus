// Member page (`/members/<id>`, `/me`) wording: the profile story.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const memberMessages = defineDashboardMessages({
  // Hero
  'peerbonus.member.directoryProfile': { defaultMessage: 'Directory profile', translations: { ja: 'Directory のプロフィール' } },
  'peerbonus.member.groups': { defaultMessage: 'Directory groups', translations: { ja: 'Directory のグループ' } },
  'peerbonus.member.pendingHint': {
    defaultMessage: 'Has not signed in yet. Points sent now are kept for them.',
    translations: { ja: 'まだサインインしていません。いま贈った pt もこの人に届きます。' },
  },
  'peerbonus.member.send': { defaultMessage: 'Give points', translations: { ja: '贈る' } },
  'peerbonus.member.sendLabel': { defaultMessage: 'Send to {name}', translations: { ja: '{name} さんに贈る' } },
  'peerbonus.member.sendFirst': { defaultMessage: 'Send your first post', translations: { ja: '最初の 1 件を贈る' } },
  'peerbonus.member.openTimeline': { defaultMessage: 'Open the timeline', translations: { ja: 'タイムラインを見る' } },

  // This week (own page)
  'peerbonus.member.weekRing': { defaultMessage: '{remaining} of {allowance} pt left this week', translations: { ja: '今週の残り {remaining} / {allowance} pt' } },
  'peerbonus.member.weekAllowance': { defaultMessage: '{allowance} pt every week', translations: { ja: '毎週 {allowance} pt 配布' } },

  // Numbers
  'peerbonus.member.stats': { defaultMessage: 'Points and posts', translations: { ja: 'ポイントと投稿' } },
  'peerbonus.member.monthReceived': { defaultMessage: 'Received this month', translations: { ja: '今月もらった' } },
  'peerbonus.member.totalReceived': { defaultMessage: 'Received in total', translations: { ja: '累計でもらった' } },
  'peerbonus.member.receivedPosts': { defaultMessage: 'Posts received', translations: { ja: 'もらった投稿' } },
  'peerbonus.member.sentPosts': { defaultMessage: 'Posts sent', translations: { ja: '贈った投稿' } },
  'peerbonus.member.postsUnit': { defaultMessage: 'posts', translations: { ja: '件' } },
  'peerbonus.member.postCount': { defaultMessage: '{count} posts', translations: { ja: '{count} 件' } },

  // Last six months
  'peerbonus.member.months': { defaultMessage: 'Last 6 months', translations: { ja: '最近 6 か月' } },
  'peerbonus.member.monthsLead': { defaultMessage: 'Points received each month and posts sent.', translations: { ja: '毎月もらった pt と、贈った投稿の数です。' } },
  'peerbonus.member.monthBar': {
    defaultMessage: '{month}: received {points} pt, sent {sent} posts',
    translations: { ja: '{month}: もらった {points} pt、贈った投稿 {sent} 件' },
  },
  'peerbonus.member.monthSent': { defaultMessage: 'sent {count}', translations: { ja: '贈った {count}' } },
  'peerbonus.member.monthSentHint': { defaultMessage: 'Posts sent that month', translations: { ja: 'その月に贈った投稿の数' } },

  // Partners
  'peerbonus.member.sentTo': { defaultMessage: 'Sends most to', translations: { ja: 'よく贈る相手' } },
  'peerbonus.member.receivedFrom': { defaultMessage: 'Receives most from', translations: { ja: 'よく贈ってくれる人' } },
  'peerbonus.member.partnersLead': { defaultMessage: 'Top 5 by points, all time', translations: { ja: '累計 pt の多い順に 5 人' } },
  'peerbonus.member.sentToEmptyMe': { defaultMessage: "You haven't sent any points yet.", translations: { ja: 'まだ誰にも贈っていません。' } },
  'peerbonus.member.sentToEmpty': { defaultMessage: "{name} hasn't sent any points yet.", translations: { ja: '{name} さんはまだ誰にも贈っていません。' } },
  'peerbonus.member.receivedFromEmptyMe': {
    defaultMessage: 'Nobody has sent you points yet. Claps and posts on the timeline get things going.',
    translations: { ja: 'まだ誰からも届いていません。タイムラインで拍手や投稿をするところから始めましょう。' },
  },
  'peerbonus.member.receivedFromEmpty': {
    defaultMessage: "{name} hasn't received points yet. Be the first.",
    translations: { ja: '{name} さんはまだ受け取っていません。最初に贈ってみませんか。' },
  },

  // Values
  'peerbonus.member.values': { defaultMessage: 'Values received', translations: { ja: 'もらったバリュー' } },
  'peerbonus.member.valuesLead': { defaultMessage: 'Values on the posts received, most first.', translations: { ja: 'もらった投稿に付いたバリューの多い順です。' } },
  'peerbonus.member.valuesEmpty': { defaultMessage: 'No received post carries a value yet.', translations: { ja: 'バリュー付きでもらった投稿はまだありません。' } },
  'peerbonus.member.valueOpen': { defaultMessage: '{value}: {count} posts. Open its posts', translations: { ja: '{value}: {count} 件。このバリューの投稿を見る' } },

  // Posts
  'peerbonus.member.tabs': { defaultMessage: 'Which posts to show', translations: { ja: '表示する投稿' } },
  'peerbonus.member.tabReceived': { defaultMessage: 'Received', translations: { ja: 'もらった' } },
  'peerbonus.member.tabSent': { defaultMessage: 'Sent', translations: { ja: '贈った' } },
  'peerbonus.member.emptyReceivedMe': { defaultMessage: 'No posts received yet', translations: { ja: 'もらった投稿はまだありません' } },
  'peerbonus.member.emptyReceived': { defaultMessage: "{name} hasn't received a post yet", translations: { ja: '{name} さんがもらった投稿はまだありません' } },
  'peerbonus.member.emptySentMe': { defaultMessage: 'No posts sent yet', translations: { ja: '贈った投稿はまだありません' } },
  'peerbonus.member.emptySent': { defaultMessage: "{name} hasn't sent a post yet", translations: { ja: '{name} さんが贈った投稿はまだありません' } },
})
