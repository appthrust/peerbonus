// 受信箱: received posts and claps on the caller's posts.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const inboxMessages = defineDashboardMessages({
  'peerbonus.inbox.leadPost': { defaultMessage: '{name} sent you this', translations: { ja: '{name} さんから届きました' } },
  'peerbonus.inbox.leadClap': { defaultMessage: '{name} clapped', translations: { ja: '{name} さんが拍手しました' } },
  'peerbonus.inbox.unread': { defaultMessage: 'Unread', translations: { ja: '未読' } },
  'peerbonus.inbox.empty.title': { defaultMessage: 'Nothing here yet', translations: { ja: 'まだ何も届いていません' } },
  'peerbonus.inbox.empty.body': {
    defaultMessage: 'Points sent to you and claps on your posts show up here. Thanking someone first is a good start.',
    translations: { ja: '届いたポイントや、投稿への拍手がここに並びます。まずはだれかに感謝を贈ってみましょう。' },
  },
  'peerbonus.inbox.end': { defaultMessage: "That's everything", translations: { ja: 'これですべてです' } },
})
