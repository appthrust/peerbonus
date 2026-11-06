// The send form v2: launcher, form and recipient picker wording. Older
// `peerbonus.compose.*` ids (copy/posts.ts) stay where their wording did not change.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const composeMessages = defineDashboardMessages({
  'peerbonus.composer.title': { defaultMessage: 'Send thanks', translations: { ja: '感謝を贈る' } },
  'peerbonus.composer.launcher': { defaultMessage: 'Thank someone…', translations: { ja: 'だれかに感謝を贈る…' } },
  'peerbonus.composer.collapse': { defaultMessage: 'Close the form', translations: { ja: 'フォームを閉じる' } },
  'peerbonus.composer.after': { defaultMessage: '{remaining} pt left this week after sending', translations: { ja: '送ると今週あと {remaining} pt' } },
  'peerbonus.composer.optional': { defaultMessage: 'Optional', translations: { ja: '任意' } },
  'peerbonus.composer.valuesMore': { defaultMessage: 'More ({count})', translations: { ja: 'もっと（{count}）' } },
  'peerbonus.composer.valuesLess': { defaultMessage: 'Fewer', translations: { ja: '少なく表示' } },
  'peerbonus.composer.sent': { defaultMessage: 'Sent {points} pt to {name}', translations: { ja: '{name} さんに {points} pt 贈りました' } },
  'peerbonus.composer.recent': { defaultMessage: 'Recent', translations: { ja: '最近' } },
  'peerbonus.composer.everyone': { defaultMessage: 'Everyone', translations: { ja: '全員' } },
  'peerbonus.composer.showEveryone': { defaultMessage: 'See everyone ({count})', translations: { ja: '全員を見る（{count}）' } },
  'peerbonus.composer.hideEveryone': { defaultMessage: 'Hide everyone', translations: { ja: '全員を閉じる' } },
  'peerbonus.composer.pendingTip': {
    defaultMessage: "Hasn't signed in yet; the points reach them at first sign-in",
    translations: { ja: '未ログイン。初回サインイン後に届きます' },
  },
})
