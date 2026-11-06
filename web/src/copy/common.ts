// Shell, navigation, feedback and error wording shared by every screen.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const commonMessages = defineDashboardMessages({
  'peerbonus.product': { defaultMessage: 'Peer Bonus', translations: { ja: 'ピアボーナス' } },
  'peerbonus.language.ja': { defaultMessage: '日本語', translations: { ja: '日本語' } },
  'peerbonus.language.en': { defaultMessage: 'English', translations: { ja: 'English' } },
  'peerbonus.nav.label': { defaultMessage: 'Peer Bonus sections', translations: { ja: 'ピアボーナスのメニュー' } },
  'peerbonus.nav.home': { defaultMessage: 'Timeline', translations: { ja: 'タイムライン' } },
  'peerbonus.nav.admin': { defaultMessage: 'Admin', translations: { ja: '管理' } },
  'peerbonus.nav.inbox': { defaultMessage: 'Inbox', translations: { ja: '受信箱' } },
  'peerbonus.nav.unread': { defaultMessage: '{count} unread', translations: { ja: '未読 {count} 件' } },
  'peerbonus.nav.balanceLeft': { defaultMessage: '{remaining} pt left', translations: { ja: 'あと {remaining} pt' } },
  'peerbonus.nav.you': { defaultMessage: 'You', translations: { ja: 'あなた' } },
  'peerbonus.nav.language': { defaultMessage: 'Display language', translations: { ja: '表示言語' } },
  'peerbonus.nav.skipToMain': { defaultMessage: 'Skip to main content', translations: { ja: '本文へ移動' } },
  'peerbonus.nav.balance': { defaultMessage: '{remaining} pt left this week', translations: { ja: '今週あと {remaining} pt' } },
  'peerbonus.common.loading': { defaultMessage: 'Loading…', translations: { ja: '読み込んでいます…' } },
  'peerbonus.common.loadFailed': { defaultMessage: "Couldn't load this page", translations: { ja: '読み込めませんでした' } },
  'peerbonus.common.retry': { defaultMessage: 'Try again', translations: { ja: 'もう一度読み込む' } },
  'peerbonus.common.loadMore': { defaultMessage: 'Load more', translations: { ja: 'もっと読む' } },
  'peerbonus.common.accessDenied': {
    defaultMessage: 'This account cannot use Peer Bonus. Sign in again or ask an administrator.',
    translations: { ja: 'このアカウントではピアボーナスを使えません。サインインし直すか、管理者に確認してください。' },
  },
  'peerbonus.common.actionFailed': { defaultMessage: "Couldn't complete the action", translations: { ja: '操作を完了できませんでした' } },
  'peerbonus.common.close': { defaultMessage: 'Close', translations: { ja: '閉じる' } },
  'peerbonus.common.cancel': { defaultMessage: 'Cancel', translations: { ja: 'キャンセル' } },
  'peerbonus.common.required': { defaultMessage: 'Required', translations: { ja: '必須' } },
  'peerbonus.common.breadcrumbs': { defaultMessage: 'Breadcrumbs', translations: { ja: 'パンくずリスト' } },
  'peerbonus.common.points': { defaultMessage: '{points} pt', translations: { ja: '{points} pt' } },
  'peerbonus.common.pointsUnit': { defaultMessage: 'pt', translations: { ja: 'pt' } },
  'peerbonus.common.pending': { defaultMessage: 'Not signed in yet', translations: { ja: '未ログイン' } },
  'peerbonus.common.resetsInDays': { defaultMessage: 'Resets in {days} days', translations: { ja: 'リセットまであと {days} 日' } },
  'peerbonus.common.resetsInHours': { defaultMessage: 'Resets in {hours} hours', translations: { ja: 'リセットまであと {hours} 時間' } },
  'peerbonus.errors.transient': {
    defaultMessage: 'Peer Bonus is temporarily unavailable. Try again in a moment.',
    translations: { ja: 'ピアボーナスに一時的につながりません。少し待ってからもう一度お試しください。' },
  },
  'peerbonus.errors.storeUnavailable': {
    defaultMessage: "Peer Bonus's database is temporarily unavailable. Try again in a moment.",
    translations: { ja: 'ピアボーナスのデータベースに一時的につながりません。少し待ってからもう一度お試しください。' },
  },
  'peerbonus.errors.identityUnavailable': {
    defaultMessage: 'Sign-in could not be checked right now. Try again in a moment.',
    translations: { ja: 'サインインを確認できませんでした。少し待ってからもう一度お試しください。' },
  },
  'peerbonus.errors.directoryUnavailable': {
    defaultMessage: 'Directory is not reachable right now, so people cannot be looked up. Try again in a moment.',
    translations: { ja: 'Directory に一時的につながらないため、メンバーを確認できません。少し待ってからもう一度お試しください。' },
  },
  'peerbonus.errors.identityConflict': {
    defaultMessage: 'Directory could not match your sign-in to a person. Ask an administrator to check your Directory entry.',
    translations: { ja: 'Directory があなたのサインインを人物に結び付けられませんでした。管理者に Directory の登録を確認してもらってください。' },
  },
  'peerbonus.errors.forbidden': { defaultMessage: "You don't have permission for this action.", translations: { ja: 'この操作をする権限がありません。' } },
  'peerbonus.errors.notFound': {
    defaultMessage: 'This post no longer exists. It may have been deleted.',
    translations: { ja: 'この投稿はもうありません。削除された可能性があります。' },
  },
  'peerbonus.errors.invalid': { defaultMessage: 'Check the entered values.', translations: { ja: '入力内容を確認してください。' } },
  'peerbonus.errors.ownPost': {
    defaultMessage: "You can't clap for a post you sent or received.",
    translations: { ja: '自分が贈った・もらった投稿には拍手できません。' },
  },
  'peerbonus.errors.selfTarget': { defaultMessage: "You can't send points to yourself.", translations: { ja: '自分にはポイントを贈れません。' } },
  'peerbonus.errors.unknownValue': {
    defaultMessage: 'One of the values no longer exists. Reload and choose again.',
    translations: { ja: '選んだバリューの一部がなくなりました。読み込み直して選び直してください。' },
  },
  'peerbonus.errors.recipientNotFound': {
    defaultMessage: 'This person is no longer in Directory. Choose someone else.',
    translations: { ja: 'この人は Directory にもういません。別の人を選んでください。' },
  },
  'peerbonus.errors.periodClosed': {
    defaultMessage: "This month's statement is closed, so its points can no longer change.",
    translations: { ja: 'この月は締め済みのため、ポイントを動かせません。' },
  },
  'peerbonus.errors.insufficientPoints': {
    defaultMessage: 'Not enough points left this week: {remaining} pt left, {required} pt needed.',
    translations: { ja: '今週の残りポイントが足りません（残り {remaining} pt、必要 {required} pt）。' },
  },
  'peerbonus.errors.insufficientPointsPlain': {
    defaultMessage: 'Not enough points left this week.',
    translations: { ja: '今週の残りポイントが足りません。' },
  },
  'peerbonus.notFound.title': { defaultMessage: 'Page not found', translations: { ja: 'ページが見つかりません' } },
  'peerbonus.notFound.body': {
    defaultMessage: 'The post or member may have been removed, or the address may be wrong.',
    translations: { ja: 'この投稿やメンバーは削除されたか、アドレスが間違っている可能性があります。' },
  },
  'peerbonus.notFound.home': { defaultMessage: 'Back to the timeline', translations: { ja: 'タイムラインへ戻る' } },
})
