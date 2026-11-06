// Admin wording (`/admin*`): participation, insights, monthly statements and
// program settings. Only Team Admins reach these screens.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const adminMessages = defineDashboardMessages({
  'peerbonus.admin.tabsLabel': { defaultMessage: 'Admin sections', translations: { ja: '管理のメニュー' } },
  'peerbonus.admin.tab.members': { defaultMessage: 'Participation', translations: { ja: '参加状況' } },
  'peerbonus.admin.tab.insights': { defaultMessage: 'Insights', translations: { ja: 'インサイト' } },
  'peerbonus.admin.tab.statements': { defaultMessage: 'Monthly statements', translations: { ja: '月次明細' } },
  'peerbonus.admin.tab.program': { defaultMessage: 'Settings', translations: { ja: '設定' } },
  'peerbonus.admin.forbiddenTitle': { defaultMessage: 'Admins only', translations: { ja: '管理者だけが使えます' } },
  'peerbonus.admin.forbiddenBody': {
    defaultMessage: 'Participation, insights, monthly statements and settings are for Team admins. Ask an admin if you need them.',
    translations: { ja: '参加状況・インサイト・月次明細・設定は Team の管理者向けです。必要な場合は管理者に依頼してください。' },
  },
  'peerbonus.admin.pendingHint': {
    defaultMessage: 'Registered in Directory but has never signed in. Points stay with this person.',
    translations: { ja: 'Directory に登録済みで、まだ一度もサインインしていない人です。pt はこの人に残ります。' },
  },

  // /admin/program
  'peerbonus.admin.program.title': { defaultMessage: 'Program settings', translations: { ja: '設定' } },
  'peerbonus.admin.program.lead': {
    defaultMessage: 'Weekly allowance, per-post limit, values and the payout rate for this Project.',
    translations: { ja: 'この Project の週の配布 pt、1 投稿の上限、バリュー、支払い換算を決めます。' },
  },
  'peerbonus.admin.program.updatedAt': { defaultMessage: 'Last saved {date}', translations: { ja: '最終保存 {date}' } },
  'peerbonus.admin.program.allowanceSection': { defaultMessage: 'Weekly points', translations: { ja: '週の配布' } },
  'peerbonus.admin.program.allowanceLead': {
    defaultMessage:
      'Changes apply from the next week nobody has given in yet. Once someone gives or claps in a week, that week keeps the values it started with.',
    translations: {
      ja: '変更は、まだ誰も贈っていない週から効きます。その週に最初の送付や拍手があった時点で値が固定され、週の途中では変わりません。',
    },
  },
  'peerbonus.admin.program.weeklyAllowance': { defaultMessage: 'Points per person per week', translations: { ja: '1 人あたりの週の配布 pt' } },
  'peerbonus.admin.program.weeklyAllowanceHint': {
    defaultMessage: '0–10000. Unused points do not carry over.',
    translations: { ja: '0〜10000。使い残しは翌週に繰り越しません。' },
  },
  'peerbonus.admin.program.maxPointsPerPost': { defaultMessage: 'Maximum points per post', translations: { ja: '1 投稿の上限 pt' } },
  'peerbonus.admin.program.maxPointsPerPostHint': {
    defaultMessage: '1 up to the weekly points ({max}).',
    translations: { ja: '1 から週の配布 pt（{max}）まで。' },
  },
  'peerbonus.admin.program.timeZoneSection': { defaultMessage: 'Time zone', translations: { ja: 'タイムゾーン' } },
  'peerbonus.admin.program.timeZoneLead': {
    defaultMessage: 'Weeks start Monday 00:00 and months on the 1st at 00:00 in this time zone.',
    translations: { ja: '週は月曜 0:00、月は 1 日 0:00 で区切ります。その基準になるタイムゾーンです。' },
  },
  'peerbonus.admin.program.timeZone': { defaultMessage: 'Time zone (IANA)', translations: { ja: 'タイムゾーン（IANA）' } },
  'peerbonus.admin.program.timeZoneHint': {
    defaultMessage: 'For example Asia/Tokyo. It locks once the first points are recorded.',
    translations: { ja: '例: Asia/Tokyo。最初の pt が記録されると変更できなくなります。' },
  },
  'peerbonus.admin.program.timeZoneLocked': {
    defaultMessage:
      'Locked: points have already been recorded. Moving week and month boundaries now would change past balances and statements.',
    translations: {
      ja: '固定されています: すでに pt が記録されています。いま週と月の区切りを動かすと、過去の残高や明細と食い違うためです。',
    },
  },
  'peerbonus.admin.program.payoutSection': { defaultMessage: 'Payout rate', translations: { ja: '支払い換算' } },
  'peerbonus.admin.program.payoutLead': {
    defaultMessage: 'Applies from the next month you close. Months already closed keep the rate they were closed with.',
    translations: { ja: '次に締める月から効きます。締め済みの月は、締めたときの単価のままです。' },
  },
  'peerbonus.admin.program.pointUnitValue': { defaultMessage: 'Amount per point', translations: { ja: '1 pt あたりの金額' } },
  'peerbonus.admin.program.pointUnitValueHint': {
    defaultMessage: 'A number of 0 or more, such as 1 or 0.5.',
    translations: { ja: '0 以上の数。例: 1、0.5' },
  },
  'peerbonus.admin.program.currency': { defaultMessage: 'Currency', translations: { ja: '通貨' } },
  'peerbonus.admin.program.currencyHint': {
    defaultMessage: 'ISO 4217 code, such as JPY or USD.',
    translations: { ja: 'ISO 4217 の通貨コード。例: JPY、USD' },
  },
  'peerbonus.admin.program.valuesLead': {
    defaultMessage:
      'Up to 3 can be attached to a post. Changes apply immediately; past posts keep their keys, and a removed key shows as the key itself.',
    translations: {
      ja: '投稿に 3 つまで付けられます。変更はすぐに反映されます。過去の投稿は key を保ち、削除した key はそのまま key で表示されます。',
    },
  },
  'peerbonus.admin.program.valuesCount': { defaultMessage: '{count} of {max} values', translations: { ja: '{count} / {max}' } },
  'peerbonus.admin.program.valuesEmpty': {
    defaultMessage: 'No values yet. Posts can still be sent without them.',
    translations: { ja: 'バリューはまだありません。バリューなしでも贈れます。' },
  },
  'peerbonus.admin.program.valueRow': { defaultMessage: 'Value {index}', translations: { ja: 'バリュー {index}' } },
  'peerbonus.admin.program.valueKey': { defaultMessage: 'Key', translations: { ja: 'key' } },
  'peerbonus.admin.program.valueKeyHint': {
    defaultMessage: 'Made from the label. Lowercase letters, digits and hyphens, up to 32. Posts store this.',
    translations: { ja: '表示名から自動で作られます。英小文字・数字・ハイフンで 32 文字まで。投稿にはこれが残ります。' },
  },
  'peerbonus.admin.program.valueKeySavedHint': {
    defaultMessage: 'Posts store this key. Past posts keep the old key if you change it.',
    translations: { ja: '投稿に残る key です。変えても、過去の投稿は元の key のまま表示されます。' },
  },
  'peerbonus.admin.program.keyShown': { defaultMessage: 'key: {key}', translations: { ja: 'key: {key}' } },
  'peerbonus.admin.program.keyDetails': { defaultMessage: 'Advanced', translations: { ja: '詳細' } },
  'peerbonus.admin.program.removeTitle': { defaultMessage: 'Remove “{label}”?', translations: { ja: '「{label}」を削除しますか？' } },
  'peerbonus.admin.program.removeBody': {
    defaultMessage: 'Past posts keep it as “#{key}”. Nothing changes until you save.',
    translations: { ja: '過去の投稿には「#{key}」として残ります。保存するまで確定しません。' },
  },
  'peerbonus.admin.program.removeConfirm': { defaultMessage: 'Remove', translations: { ja: '削除' } },
  'peerbonus.admin.program.valueLabel': { defaultMessage: 'Label', translations: { ja: '表示名' } },
  'peerbonus.admin.program.valueDescription': { defaultMessage: 'Description (optional)', translations: { ja: '説明（任意）' } },
  'peerbonus.admin.program.addValue': { defaultMessage: 'Add value', translations: { ja: 'バリューを追加' } },
  'peerbonus.admin.program.removeValue': { defaultMessage: 'Remove value {index}', translations: { ja: 'バリュー {index} を削除' } },
  'peerbonus.admin.program.moveUp': { defaultMessage: 'Move value {index} up', translations: { ja: 'バリュー {index} を上へ' } },
  'peerbonus.admin.program.moveDown': { defaultMessage: 'Move value {index} down', translations: { ja: 'バリュー {index} を下へ' } },
  'peerbonus.admin.program.save': { defaultMessage: 'Save', translations: { ja: '保存' } },
  'peerbonus.admin.program.discard': { defaultMessage: 'Discard changes', translations: { ja: '変更を取り消す' } },
  'peerbonus.admin.program.unsaved': { defaultMessage: 'Unsaved changes', translations: { ja: '未保存の変更があります' } },
  'peerbonus.admin.program.saved': { defaultMessage: 'Settings saved.', translations: { ja: '設定を保存しました。' } },
  'peerbonus.admin.program.fixErrors': {
    defaultMessage: 'Fix the highlighted fields before saving.',
    translations: { ja: '赤く示した項目を直してから保存してください。' },
  },
  'peerbonus.admin.program.leave': {
    defaultMessage: 'Leave without saving? Your changes will be lost.',
    translations: { ja: '保存せずに移動しますか？変更は失われます。' },
  },
  'peerbonus.admin.program.timezoneLockedError': {
    defaultMessage: 'Points were recorded meanwhile, so the time zone is now locked and was put back. Save again to keep your other changes.',
    translations: { ja: 'その間に pt が記録されたため、タイムゾーンは固定され元に戻しました。ほかの変更を残すにはもう一度保存してください。' },
  },
  'peerbonus.admin.program.error.timeZone': {
    defaultMessage: 'Enter an IANA time zone, such as Asia/Tokyo.',
    translations: { ja: 'IANA のタイムゾーン名を入力してください（例: Asia/Tokyo）。' },
  },
  'peerbonus.admin.program.error.weeklyAllowance': {
    defaultMessage: 'Enter a whole number from 0 to 10000.',
    translations: { ja: '0〜10000 の整数を入力してください。' },
  },
  'peerbonus.admin.program.error.maxPointsPerPost': {
    defaultMessage: 'Enter a whole number from 1 to {max}.',
    translations: { ja: '1〜{max} の整数を入力してください。' },
  },
  'peerbonus.admin.program.error.maxPointsNoAllowance': {
    defaultMessage: 'Set the weekly points to 1 or more first.',
    translations: { ja: '先に週の配布 pt を 1 以上にしてください。' },
  },
  'peerbonus.admin.program.error.pointUnitValue': {
    defaultMessage: 'Enter a number of 0 or more, such as 1 or 0.5.',
    translations: { ja: '0 以上の数を入力してください（例: 1、0.5）。' },
  },
  'peerbonus.admin.program.error.currency': {
    defaultMessage: 'Enter a three-letter uppercase code, such as JPY.',
    translations: { ja: '大文字 3 文字の通貨コードを入力してください（例: JPY）。' },
  },
  'peerbonus.admin.program.error.valueKey': {
    defaultMessage: 'Use 1–32 lowercase letters, digits or hyphens.',
    translations: { ja: '英小文字・数字・ハイフンで 1〜32 文字にしてください。' },
  },
  'peerbonus.admin.program.error.valueKeyDuplicate': {
    defaultMessage: 'Another value already uses this key.',
    translations: { ja: '同じ key がほかの行にあります。' },
  },
  'peerbonus.admin.program.error.valueLabel': { defaultMessage: 'Enter a label.', translations: { ja: '表示名を入力してください。' } },

  // /admin/statements
  'peerbonus.admin.statements.lead': {
    defaultMessage: 'Points received per month and what they pay out. Close months oldest first, then hand the CSV to payroll.',
    translations: { ja: '月ごとの受取 pt と支払額です。古い月から順に締め、締めた月の CSV を給与担当に渡します。' },
  },
  'peerbonus.admin.statements.preview': { defaultMessage: '{month} so far', translations: { ja: '{month}の見込み' } },
  'peerbonus.admin.statements.previewLead': {
    defaultMessage: 'An estimate until the month is closed, at the current rate of {rate} per point.',
    translations: { ja: '締めるまでの見込みです。単価は現在の設定値（1 pt = {rate}）で計算しています。' },
  },
  'peerbonus.admin.statements.months': { defaultMessage: 'Months', translations: { ja: '月の一覧' } },
  'peerbonus.admin.col.month': { defaultMessage: 'Month', translations: { ja: '月' } },
  'peerbonus.admin.statements.col.status': { defaultMessage: 'Status', translations: { ja: '状態' } },
  'peerbonus.admin.statements.col.actions': { defaultMessage: 'Actions', translations: { ja: '操作' } },
  'peerbonus.admin.statements.status.closed': { defaultMessage: 'Closed', translations: { ja: '締め済み' } },
  'peerbonus.admin.statements.status.open': { defaultMessage: 'Open', translations: { ja: '未締め' } },
  'peerbonus.admin.statements.status.current': { defaultMessage: 'This month', translations: { ja: '今月' } },
  'peerbonus.admin.statements.view': { defaultMessage: 'Details', translations: { ja: '明細' } },
  'peerbonus.admin.statements.viewMonth': { defaultMessage: 'Details for {month}', translations: { ja: '{month}の明細' } },
  'peerbonus.admin.statements.csv': { defaultMessage: 'CSV', translations: { ja: 'CSV' } },
  'peerbonus.admin.statements.csvMonth': { defaultMessage: 'Download the CSV for {month}', translations: { ja: '{month}の CSV をダウンロード' } },
  'peerbonus.admin.statements.csvDownload': { defaultMessage: 'Download CSV', translations: { ja: 'CSV をダウンロード' } },
  'peerbonus.admin.statements.csvNote': {
    defaultMessage: 'Clap points count in the month of the clap: a clap in November on an October post is in November.',
    translations: { ja: '拍手の pt は拍手した月に入ります。10 月の投稿に 11 月に拍手すると 11 月の明細に載ります。' },
  },
  'peerbonus.admin.statements.close': { defaultMessage: 'Close month', translations: { ja: '締める' } },
  'peerbonus.admin.statements.closeMonth': { defaultMessage: 'Close {month}', translations: { ja: '{month}を締める' } },
  'peerbonus.admin.statements.confirmTitle': { defaultMessage: 'Close {month}?', translations: { ja: '{month}を締めますか？' } },
  'peerbonus.admin.statements.confirmBody': {
    defaultMessage:
      'The statement for {month} is frozen at {rate} per point and cannot be reopened. Claps on its posts and point returns from deleting its posts stop.',
    translations: {
      ja: '{month}の明細を 1 pt = {rate} で確定します。締めた月は再オープンできません。この月の投稿への拍手と、削除による pt の戻しもできなくなります。',
    },
  },
  'peerbonus.admin.statements.closed': { defaultMessage: '{month} is closed.', translations: { ja: '{month}を締めました。' } },
  'peerbonus.admin.statements.error.monthNotEnded': {
    defaultMessage: 'This month has not ended yet in the program time zone.',
    translations: { ja: 'Program のタイムゾーンでは、この月はまだ終わっていません。' },
  },
  'peerbonus.admin.statements.error.previousMonthOpen': {
    defaultMessage: 'An earlier month with points is still open. Close it first.',
    translations: { ja: 'pt のある前の月がまだ締められていません。先にそちらを締めてください。' },
  },
  'peerbonus.admin.statements.error.invalidMonth': { defaultMessage: 'That month is not valid.', translations: { ja: '月の指定が正しくありません。' } },
  'peerbonus.admin.statements.empty': { defaultMessage: 'No months yet', translations: { ja: 'まだ月がありません' } },
  'peerbonus.admin.statements.emptyBody': {
    defaultMessage: 'Months appear from the first points given.',
    translations: { ja: '最初に pt が贈られた月から表示されます。' },
  },

  // /admin/statements/:month
  'peerbonus.admin.statement.title': { defaultMessage: 'Statement for {month}', translations: { ja: '{month}の明細' } },
  'peerbonus.admin.statement.closedBy': {
    defaultMessage: 'Closed {date} by {name}',
    translations: { ja: '{date} に {name} が締めました' },
  },
  'peerbonus.admin.statement.closedAt': { defaultMessage: 'Closed {date}', translations: { ja: '{date} に締めました' } },
  'peerbonus.admin.statement.openLead': {
    defaultMessage: 'Not closed yet: an estimate at the current rate that changes until the month is closed.',
    translations: { ja: 'まだ締めていません。現在の単価で計算した見込みで、締めるまで変わります。' },
  },
  'peerbonus.admin.statement.totalPoints': { defaultMessage: 'Total points', translations: { ja: '合計 pt' } },
  'peerbonus.admin.statement.totalAmount': { defaultMessage: 'Total amount', translations: { ja: '支払額の合計' } },
  'peerbonus.admin.statement.memberCount': { defaultMessage: 'People', translations: { ja: '対象人数' } },
  'peerbonus.admin.statement.rate': { defaultMessage: 'Rate', translations: { ja: '単価' } },
  'peerbonus.admin.statement.perPoint': { defaultMessage: '/ pt', translations: { ja: '/ pt' } },
  'peerbonus.admin.statement.people': { defaultMessage: '{count} people', translations: { ja: '{count} 人' } },
  'peerbonus.admin.col.name': { defaultMessage: 'Name', translations: { ja: '名前' } },
  'peerbonus.admin.statement.col.amount': { defaultMessage: 'Amount', translations: { ja: '金額' } },
  'peerbonus.admin.statement.total': { defaultMessage: 'Total', translations: { ja: '合計' } },
  'peerbonus.admin.statement.lines': { defaultMessage: 'Recipients', translations: { ja: '受け取った人' } },
  'peerbonus.admin.statement.empty': { defaultMessage: 'Nobody received points this month', translations: { ja: 'この月に pt を受け取った人はいません' } },
  'peerbonus.admin.statement.directoryStale': {
    defaultMessage: "Names and emails couldn't be refreshed from Directory at close",
    translations: { ja: '締めるときに Directory から名前とメールを取り直せませんでした' },
  },
  'peerbonus.admin.statement.directoryStaleBody': {
    defaultMessage: 'This statement uses the last known names and emails. Check them before handing the CSV to payroll.',
    translations: { ja: 'この明細は最後に保存された名前とメールのままです。CSV を給与担当に渡す前に確認してください。' },
  },
  'peerbonus.admin.statement.checklist': { defaultMessage: 'Before closing', translations: { ja: '締める前の確認' } },
  'peerbonus.admin.statement.check.ok': { defaultMessage: 'Done:', translations: { ja: '済み:' } },
  'peerbonus.admin.statement.check.missing': { defaultMessage: 'Not yet:', translations: { ja: '未完了:' } },
  'peerbonus.admin.statement.check.ended': { defaultMessage: 'The month has ended', translations: { ja: '月が終わっている' } },
  'peerbonus.admin.statement.check.notEnded': {
    defaultMessage: 'The month has not ended (it can be closed from {date})',
    translations: { ja: '月がまだ終わっていません（{date} から締められます）' },
  },
  'peerbonus.admin.statement.check.previousClosed': { defaultMessage: 'Every earlier month is closed', translations: { ja: '前の月がすべて締まっている' } },
  'peerbonus.admin.statement.check.previousOpen': {
    defaultMessage: 'An earlier month ({month}) is still open. Months close oldest first.',
    translations: { ja: '前の月（{month}）がまだ締まっていません。古い月から順に締めます。' },
  },
  'peerbonus.admin.statement.check.openPrevious': { defaultMessage: 'Go to {month}', translations: { ja: '{month}の明細へ' } },
  'peerbonus.admin.statement.previewNames': {
    defaultMessage: 'Names and emails are the last known ones; they are refreshed from Directory when the month is closed.',
    translations: { ja: '名前とメールは最後に保存された値です。締めるときに Directory から取り直します。' },
  },

  // /admin/members
  'peerbonus.admin.participation.lead': {
    defaultMessage: 'Everyone in Directory and what they did in {month}, most points received first.',
    translations: { ja: '{month}に Directory の全員が何をしたか。もらった pt の多い順です。' },
  },
  'peerbonus.admin.col.receivedPoints': { defaultMessage: 'Points received', translations: { ja: 'もらった pt' } },
  'peerbonus.admin.participation.col.sentPoints': { defaultMessage: 'Points sent', translations: { ja: '贈った pt' } },
  'peerbonus.admin.participation.col.clapsGiven': { defaultMessage: 'Claps given', translations: { ja: '拍手' } },
  'peerbonus.admin.participation.people': { defaultMessage: 'In Directory', translations: { ja: '対象' } },
  'peerbonus.admin.participation.peopleUnit': { defaultMessage: 'people', translations: { ja: '人' } },
  'peerbonus.admin.participation.senders': { defaultMessage: 'Senders', translations: { ja: '贈った' } },
  'peerbonus.admin.participation.recipients': { defaultMessage: 'Receivers', translations: { ja: 'もらった' } },
  'peerbonus.admin.participation.rate': { defaultMessage: 'Participation rate', translations: { ja: '参加率' } },
  'peerbonus.admin.participation.rateNote': {
    defaultMessage: '{active} took part, {inactive} not yet',
    translations: { ja: '{active} 人が参加・{inactive} 人がまだ' },
  },
  'peerbonus.admin.participation.notYet': { defaultMessage: 'Not taking part yet', translations: { ja: 'まだ参加していません' } },
  'peerbonus.admin.participation.quiet': { defaultMessage: 'Nothing this month', translations: { ja: 'この月の動きはありません' } },
  'peerbonus.admin.participation.directoryDown': { defaultMessage: "Directory can't be reached", translations: { ja: 'Directory に接続できません' } },
  'peerbonus.admin.participation.directoryDownBody': {
    defaultMessage: 'Only members with activity are listed for now; people who have not taken part appear again once Directory is back.',
    translations: { ja: 'いまは動きのあったメンバーだけを表示しています。まだ参加していない人は、Directory が戻ると再び表示されます。' },
  },
  'peerbonus.admin.participation.empty': { defaultMessage: 'Nobody to show for this month', translations: { ja: 'この月に表示できる人はいません' } },
  'peerbonus.admin.participation.emptyBody': {
    defaultMessage: 'People appear here from Directory, and members once they give or receive points.',
    translations: { ja: 'Directory の人と、pt を贈ったりもらったりしたメンバーがここに並びます。' },
  },

  // /admin/insights
  'peerbonus.admin.insights.lead': {
    defaultMessage: 'How {month} went: posts, the values people recognised and the weekly rhythm.',
    translations: { ja: '{month}の投稿、認め合われたバリュー、週ごとの動きです。' },
  },
  'peerbonus.admin.insights.posts': { defaultMessage: 'Total posts', translations: { ja: '投稿' } },
  'peerbonus.admin.insights.clapsUnit': { defaultMessage: 'times', translations: { ja: '回' } },
  'peerbonus.admin.insights.values': { defaultMessage: 'By value', translations: { ja: 'バリュー別' } },
  'peerbonus.admin.insights.valuesLead': { defaultMessage: 'Posts carrying each value, most first.', translations: { ja: 'バリューが付いた投稿の数（多い順）です。' } },
  'peerbonus.admin.insights.valuesEmpty': { defaultMessage: 'No post this month carries a value.', translations: { ja: 'この月にバリュー付きの投稿はありません。' } },
  'peerbonus.admin.insights.weeks': { defaultMessage: 'By week', translations: { ja: '週別' } },
  'peerbonus.admin.insights.weeksLead': { defaultMessage: 'Points received per week (Monday to Sunday).', translations: { ja: '週（月曜〜日曜）ごとに受け取られた pt です。' } },
  'peerbonus.admin.insights.weekDetail': { defaultMessage: '{posts} posts, {claps} claps', translations: { ja: '{posts} 件・拍手 {claps}' } },
  'peerbonus.admin.insights.topRecipients': { defaultMessage: 'Received the most', translations: { ja: 'たくさんもらった人' } },
  'peerbonus.admin.insights.topSenders': { defaultMessage: 'Gave the most', translations: { ja: 'たくさん贈った人' } },
  'peerbonus.admin.insights.nobody': { defaultMessage: 'Nobody yet.', translations: { ja: 'まだいません。' } },
  'peerbonus.admin.insights.empty': { defaultMessage: 'No posts in {month} yet', translations: { ja: '{month}の投稿はまだありません' } },
  'peerbonus.admin.insights.emptyBody': {
    defaultMessage: 'Once the first post arrives, values and the weekly rhythm show up here.',
    translations: { ja: '最初の 1 件が届くと、バリューや週ごとの動きがここに並びます。' },
  },
  'peerbonus.admin.insights.emptyAction': { defaultMessage: 'Send on the timeline', translations: { ja: 'タイムラインで贈る' } },
})
