// Slack integration wording (`/admin/slack`): connection state, the setup
// guide, delivery settings, the inbound endpoint (Phase B) and the delivery
// log. Product names (Slack, Dashboard, manifest, bot token) stay in English.
import { defineDashboardMessages } from '@appthrust/platform-dashboard-i18n'

export const slackMessages = defineDashboardMessages({
  'peerbonus.admin.tab.slack': { defaultMessage: 'Slack', translations: { ja: 'Slack' } },
  'peerbonus.slack.title': { defaultMessage: 'Slack integration', translations: { ja: 'Slack 連携' } },
  'peerbonus.slack.lead': {
    defaultMessage: 'Announce new posts in a Slack channel and send recipients a direct message.',
    translations: { ja: '新しい投稿を Slack のチャンネルに告知し、受け手に DM を送ります。' },
  },

  // Status card
  'peerbonus.slack.status.title': { defaultMessage: 'Connection', translations: { ja: '接続' } },
  'peerbonus.slack.state.unconfigured': { defaultMessage: 'Not set up', translations: { ja: '未設定' } },
  'peerbonus.slack.state.unconfiguredBody': {
    defaultMessage: 'No bot token yet. Follow steps 1 and 2 of the setup guide below.',
    translations: { ja: 'Bot token がまだありません。下のセットアップ手順の 1 と 2 を進めてください。' },
  },
  'peerbonus.slack.state.configured': { defaultMessage: 'Not verified', translations: { ja: '未確認' } },
  'peerbonus.slack.state.configuredBody': {
    defaultMessage: 'The bot token is set. Verify it to fix the Slack workspace Peer Bonus writes to.',
    translations: { ja: 'Bot token は設定済みです。確認すると、Peer Bonus が書き込む Slack ワークスペースが固定されます。' },
  },
  'peerbonus.slack.state.connected': { defaultMessage: 'Connected', translations: { ja: '接続中' } },
  'peerbonus.slack.state.connectedBody': {
    defaultMessage: 'Peer Bonus writes to this workspace only.',
    translations: { ja: 'Peer Bonus はこのワークスペースにだけ書き込みます。' },
  },
  'peerbonus.slack.state.teamChanged': { defaultMessage: 'Workspace changed', translations: { ja: 'ワークスペースが変わりました' } },
  'peerbonus.slack.state.teamChangedBody': {
    defaultMessage: 'The bot token now belongs to another workspace, so deliveries are paused. Switch to keep sending from the new workspace.',
    translations: { ja: 'Bot token が別のワークスペースのものに変わったため、配送を止めています。切り替えると新しいワークスペースで送信を再開します。' },
  },
  'peerbonus.slack.status.verifiedAt': { defaultMessage: 'Verified {date}', translations: { ja: '{date} に確認' } },
  'peerbonus.slack.status.sent24h': { defaultMessage: 'Sent, 24 h', translations: { ja: '送信済み（24 時間）' } },
  'peerbonus.slack.status.failed24h': { defaultMessage: 'Failed, 24 h', translations: { ja: '失敗（24 時間）' } },
  'peerbonus.slack.status.lastError': { defaultMessage: 'Last error {date}', translations: { ja: '直近のエラー（{date}）' } },
  'peerbonus.slack.verify': { defaultMessage: 'Verify', translations: { ja: '確認する' } },
  'peerbonus.slack.reverify': { defaultMessage: 'Verify again', translations: { ja: '再確認' } },
  'peerbonus.slack.forceVerify': { defaultMessage: 'Switch workspace…', translations: { ja: 'ワークスペースを切り替える…' } },
  'peerbonus.slack.verified': { defaultMessage: 'Verified {team}.', translations: { ja: '{team} を確認しました。' } },
  'peerbonus.slack.force.title': { defaultMessage: 'Switch the Slack workspace?', translations: { ja: 'Slack ワークスペースを切り替えますか？' } },
  'peerbonus.slack.force.body': {
    defaultMessage:
      'Peer Bonus is fixed to {current}. Switching to {next} forgets which Slack account belongs to which member and clears the announce channel. Waiting deliveries are then sent to {next}.',
    translations: {
      ja: 'Peer Bonus は {current} に固定されています。{next} に切り替えると、メンバーと Slack アカウントの対応と告知チャンネルが消えます。送信待ちの配送は {next} に送られます。',
    },
  },
  'peerbonus.slack.force.tokenWorkspace': { defaultMessage: 'the bot token’s workspace', translations: { ja: 'Bot token のワークスペース' } },
  'peerbonus.slack.force.confirm': { defaultMessage: 'Switch', translations: { ja: '切り替える' } },

  // Action errors
  'peerbonus.slack.error.notConfigured': { defaultMessage: 'No bot token is set yet.', translations: { ja: 'Bot token がまだ設定されていません。' } },
  'peerbonus.slack.error.notConnected': {
    defaultMessage: 'Slack is not connected. Verify the workspace first.',
    translations: { ja: 'Slack に接続していません。先にワークスペースを確認してください。' },
  },
  'peerbonus.slack.error.signingSecretMissing': {
    defaultMessage: 'Giving from Slack needs the Slack signing secret in the Dashboard credentials.',
    translations: { ja: 'Slack から贈るには、Dashboard の Credentials に Slack signing secret が必要です。' },
  },
  'peerbonus.slack.error.botNotInChannel': {
    defaultMessage: 'Peer Bonus is not in #{channel}. Run /invite @Peer Bonus in that channel, then send again.',
    translations: { ja: 'Peer Bonus が #{channel} に参加していません。そのチャンネルで /invite @Peer Bonus してから、もう一度送ってください。' },
  },
  'peerbonus.slack.error.slack': { defaultMessage: 'Slack answered with an error: {code}', translations: { ja: 'Slack がエラーを返しました: {code}' } },
  'peerbonus.slack.error.teamChanged': { defaultMessage: 'The bot token belongs to another workspace', translations: { ja: 'Bot token が別のワークスペースのものです' } },
  'peerbonus.slack.error.invalidAuth': { defaultMessage: 'The bot token is not valid', translations: { ja: 'Bot token が無効です' } },
  'peerbonus.slack.error.tokenRevoked': { defaultMessage: 'The bot token was revoked', translations: { ja: 'Bot token が取り消されました' } },
  'peerbonus.slack.error.notInChannel': { defaultMessage: 'The bot is not in the channel', translations: { ja: 'bot がチャンネルに参加していません' } },
  'peerbonus.slack.error.channelNotFound': {
    defaultMessage: 'The channel is gone or archived',
    translations: { ja: 'チャンネルが見つからないか、アーカイブされています' },
  },
  'peerbonus.slack.error.missingScope': {
    defaultMessage: 'The Slack app lacks a permission; update it from the manifest',
    translations: { ja: 'Slack アプリに必要な権限がありません。manifest から更新してください' },
  },
  'peerbonus.slack.error.rateLimited': { defaultMessage: 'Slack asked to slow down; retrying', translations: { ja: 'Slack の送信制限中。再試行します' } },
  'peerbonus.slack.error.userNotFound': {
    defaultMessage: 'No Slack account uses the recipient’s email',
    translations: { ja: '受け手のメールアドレスの Slack アカウントがありません' },
  },
  'peerbonus.slack.error.announceNotSent': { defaultMessage: 'The announcement was not sent', translations: { ja: '告知が送られなかったため' } },
  'peerbonus.slack.error.postDeleted': { defaultMessage: 'The post was deleted', translations: { ja: '投稿が削除されたため' } },

  // Setup guide
  'peerbonus.slack.setup.title': { defaultMessage: 'Setup guide', translations: { ja: 'セットアップ手順' } },
  'peerbonus.slack.setup.lead': {
    defaultMessage: 'Done once, by someone who administers both this Project and the Slack workspace.',
    translations: { ja: 'この Project と Slack ワークスペースの両方の管理者が、最初に 1 回だけ行います。' },
  },
  'peerbonus.slack.setup.complete': { defaultMessage: 'Setup complete', translations: { ja: 'セットアップ済み' } },
  'peerbonus.slack.setup.done': { defaultMessage: 'Done', translations: { ja: '完了' } },
  'peerbonus.slack.setup.show': { defaultMessage: 'Show steps', translations: { ja: '手順を表示' } },
  'peerbonus.slack.setup.hide': { defaultMessage: 'Hide steps', translations: { ja: '手順を隠す' } },
  'peerbonus.slack.setup.createApp': { defaultMessage: 'Create the Slack app from the manifest', translations: { ja: 'manifest から Slack アプリを作る' } },
  'peerbonus.slack.setup.createAppBody': {
    defaultMessage: 'On api.slack.com/apps choose Create New App › From a manifest and paste the downloaded file.',
    translations: { ja: 'api.slack.com/apps で Create New App › From a manifest を選び、ダウンロードしたファイルの中身を貼り付けます。' },
  },
  'peerbonus.slack.setup.openSlackApps': { defaultMessage: 'Open api.slack.com/apps', translations: { ja: 'api.slack.com/apps を開く' } },
  'peerbonus.slack.setup.downloadManifest': { defaultMessage: 'Download manifest (JSON)', translations: { ja: 'manifest をダウンロード（JSON）' } },
  'peerbonus.slack.setup.pasteToken': { defaultMessage: 'Paste the bot token in the Dashboard', translations: { ja: 'Bot token を Dashboard に貼る' } },
  'peerbonus.slack.setup.pasteTokenBody': {
    defaultMessage:
      'Install the app to the workspace and copy its Bot User OAuth Token (xoxb-…). Open the Dashboard page below, which shows the credentials of Peer Bonus, and paste it into Slack bot token. Peer Bonus restarts within about a minute.',
    translations: {
      ja: 'アプリをワークスペースにインストールし、Bot User OAuth Token（xoxb-…）をコピーします。下の Dashboard のページで Peer Bonus の認証情報を開き、Slack bot token に貼ります。約 1 分で Peer Bonus が再起動します。',
    },
  },
  'peerbonus.slack.setup.dashboardPath': { defaultMessage: 'Dashboard page', translations: { ja: 'Dashboard のページ' } },
  'peerbonus.slack.setup.dashboardPathHint': {
    defaultMessage: 'Open this path on the Dashboard where you manage this Project.',
    translations: { ja: 'この Project を管理している Dashboard で、このパスを開いてください。' },
  },
  'peerbonus.slack.setup.connect': { defaultMessage: 'Verify, choose a channel, send a test', translations: { ja: '確認し、チャンネルを選び、テスト送信する' } },
  'peerbonus.slack.setup.connectBody': {
    defaultMessage: 'Verify above, choose the announce channel below, run /invite @Peer Bonus in that channel, then send a test message.',
    translations: { ja: '上で「確認する」→ 下で告知チャンネルを選ぶ → Slack でそのチャンネルに /invite @Peer Bonus → 「テスト送信」。' },
  },
  'peerbonus.slack.setup.signingSecret': { defaultMessage: 'Let people give from Slack', translations: { ja: 'Slack から贈れるようにする' } },
  'peerbonus.slack.setup.signingSecretBody': {
    defaultMessage: 'Paste Basic Information › Signing Secret into Credentials › Slack signing secret, then turn on Give from Slack below.',
    translations: { ja: 'Basic Information › Signing Secret を Credentials › Slack signing secret に貼り、下の「Slack から贈る」をオンにします。' },
  },
  'peerbonus.slack.copy': { defaultMessage: 'Copy', translations: { ja: 'コピー' } },
  'peerbonus.slack.copied': { defaultMessage: 'Copied', translations: { ja: 'コピーしました' } },

  // Delivery settings
  'peerbonus.slack.settings.title': { defaultMessage: 'Delivery', translations: { ja: '配信設定' } },
  'peerbonus.slack.settings.lead': {
    defaultMessage: 'Slack shows posts as the timeline does (people, points, message, values). Choose a channel whose readers may see them.',
    translations: { ja: 'Slack にはタイムラインと同じ内容（送り手・受け手・pt・本文・バリュー）が出ます。読む人が見てよいチャンネルを選んでください。' },
  },
  'peerbonus.slack.settings.locked': {
    defaultMessage: 'Verify the workspace to change these settings.',
    translations: { ja: 'ワークスペースを確認すると、変更できるようになります。' },
  },
  'peerbonus.slack.channel': { defaultMessage: 'Announce channel', translations: { ja: '告知チャンネル' } },
  'peerbonus.slack.channel.hint': { defaultMessage: 'Public channels only.', translations: { ja: '公開チャンネルのみ。' } },
  'peerbonus.slack.channel.search': { defaultMessage: 'Search channels', translations: { ja: 'チャンネルを検索' } },
  'peerbonus.slack.channel.options': { defaultMessage: 'Channels', translations: { ja: 'チャンネル候補' } },
  'peerbonus.slack.channel.none': { defaultMessage: 'No public channel starts with “{q}”.', translations: { ja: '「{q}」で始まる公開チャンネルはありません。' } },
  'peerbonus.slack.channel.notJoined': { defaultMessage: 'Not joined', translations: { ja: '未参加' } },
  'peerbonus.slack.channel.change': { defaultMessage: 'Change channel', translations: { ja: '変更' } },
  'peerbonus.slack.channel.remove': { defaultMessage: 'Remove channel', translations: { ja: '外す' } },
  'peerbonus.slack.channel.joined': { defaultMessage: 'Peer Bonus is in this channel.', translations: { ja: 'Peer Bonus が参加しています。' } },
  'peerbonus.slack.channel.invite': {
    defaultMessage: 'Peer Bonus is not in this channel yet. Run /invite @Peer Bonus there.',
    translations: { ja: 'Peer Bonus はまだ参加していません。このチャンネルで /invite @Peer Bonus してください。' },
  },
  'peerbonus.slack.channel.required': { defaultMessage: 'Choose a channel to announce posts in.', translations: { ja: '告知するチャンネルを選んでください。' } },
  'peerbonus.slack.announcePosts': { defaultMessage: 'Announce new posts', translations: { ja: '新しい投稿を告知する' } },
  'peerbonus.slack.announcePostsHint': {
    defaultMessage: 'Claps update the message; deleting the post deletes it.',
    translations: { ja: '拍手でメッセージが更新され、投稿を削除すると消えます。' },
  },
  'peerbonus.slack.dmRecipientOnPost': { defaultMessage: 'Send the recipient a DM', translations: { ja: '受け手に DM を送る' } },
  'peerbonus.slack.dmRecipientOnPostHint': {
    defaultMessage: 'Matched by email; people without a Slack account are skipped.',
    translations: { ja: 'メールアドレスで Slack アカウントを探します。見つからない人には送りません。' },
  },
  'peerbonus.slack.giveFromSlack': { defaultMessage: 'Give from Slack', translations: { ja: 'Slack から贈る' } },
  'peerbonus.slack.giveFromSlackHint': {
    defaultMessage: 'The /peerbonus command and the clap button on announcements.',
    translations: { ja: '/peerbonus コマンドと、告知の拍手ボタン。' },
  },
  'peerbonus.slack.giveFromSlackLocked': {
    defaultMessage: 'Needs the Slack signing secret in the Dashboard credentials.',
    translations: { ja: 'Dashboard の Credentials に Slack signing secret が必要です。' },
  },
  'peerbonus.slack.locale': { defaultMessage: 'Message language', translations: { ja: 'メッセージの言語' } },
  'peerbonus.slack.localeHint': { defaultMessage: 'For everything Peer Bonus writes in Slack.', translations: { ja: 'Peer Bonus が Slack に書くすべてのメッセージに使います。' } },
  'peerbonus.slack.test': { defaultMessage: 'Send test message', translations: { ja: 'テスト送信' } },
  'peerbonus.slack.testSaveFirst': {
    defaultMessage: 'Save first: the test goes to the saved channel.',
    translations: { ja: '先に保存してください。テストは保存済みのチャンネルに送ります。' },
  },
  'peerbonus.slack.testSent': { defaultMessage: 'Test message sent to #{channel}.', translations: { ja: '#{channel} にテストメッセージを送りました。' } },

  // Inbound (Phase B)
  'peerbonus.slack.inbound.title': { defaultMessage: 'Requests from Slack', translations: { ja: 'Slack からのリクエスト' } },
  'peerbonus.slack.inbound.lead': {
    defaultMessage: 'Slack sends the /peerbonus command and button presses to this URL. The manifest below already points there.',
    translations: { ja: 'Slack は /peerbonus コマンドとボタン操作をこの URL に送ります。下の manifest は設定済みです。' },
  },
  'peerbonus.slack.inbound.url': { defaultMessage: 'Request URL', translations: { ja: 'リクエスト URL' } },
  'peerbonus.slack.inbound.ready': { defaultMessage: 'Ready', translations: { ja: '受付中' } },
  'peerbonus.slack.inbound.notReady': { defaultMessage: 'Not ready', translations: { ja: '準備中' } },
  'peerbonus.slack.inbound.noUrl': { defaultMessage: 'The platform has not published the URL yet.', translations: { ja: 'platform がまだ URL を公開していません。' } },
  'peerbonus.slack.inbound.reason': { defaultMessage: 'Reason: {reason}', translations: { ja: '理由: {reason}' } },
  'peerbonus.slack.inbound.download': { defaultMessage: 'Download manifest with commands (JSON)', translations: { ja: 'コマンド付き manifest をダウンロード（JSON）' } },
  'peerbonus.slack.inbound.downloadHint': {
    defaultMessage: 'In the Slack app’s settings, replace App Manifest with this file.',
    translations: { ja: 'Slack アプリの設定で、App Manifest をこのファイルの内容に置き換えます。' },
  },

  // Delivery log
  'peerbonus.slack.deliveries.title': { defaultMessage: 'Deliveries', translations: { ja: '配送履歴' } },
  'peerbonus.slack.deliveries.lead': {
    defaultMessage: 'The latest 50 messages Peer Bonus sent or tried to send.',
    translations: { ja: 'Peer Bonus が送った、または送ろうとした直近 50 件です。' },
  },
  'peerbonus.slack.deliveries.filter': { defaultMessage: 'Filter by state', translations: { ja: '状態で絞り込む' } },
  'peerbonus.slack.deliveries.all': { defaultMessage: 'All', translations: { ja: 'すべて' } },
  'peerbonus.slack.deliveries.state.pending': { defaultMessage: 'Waiting', translations: { ja: '送信待ち' } },
  'peerbonus.slack.deliveries.state.sent': { defaultMessage: 'Delivered', translations: { ja: '送信済み' } },
  'peerbonus.slack.deliveries.state.skipped': { defaultMessage: 'Skipped', translations: { ja: 'スキップ' } },
  'peerbonus.slack.deliveries.state.failed': { defaultMessage: 'Failed', translations: { ja: '失敗' } },
  'peerbonus.slack.deliveries.kind.postAnnounce': { defaultMessage: 'Announcement', translations: { ja: '告知' } },
  'peerbonus.slack.deliveries.kind.postDm': { defaultMessage: 'DM to recipient', translations: { ja: '受け手への DM' } },
  'peerbonus.slack.deliveries.kind.clapUpdate': { defaultMessage: 'Clap count', translations: { ja: '拍手数の更新' } },
  'peerbonus.slack.deliveries.kind.postDelete': { defaultMessage: 'Deletion', translations: { ja: '告知の削除' } },
  'peerbonus.slack.deliveries.col.kind': { defaultMessage: 'Kind', translations: { ja: '種類' } },
  'peerbonus.slack.deliveries.col.state': { defaultMessage: 'State', translations: { ja: '状態' } },
  'peerbonus.slack.deliveries.col.attempts': { defaultMessage: 'Attempts', translations: { ja: '試行' } },
  'peerbonus.slack.deliveries.col.created': { defaultMessage: 'Created', translations: { ja: '作成' } },
  'peerbonus.slack.deliveries.col.sent': { defaultMessage: 'Sent at', translations: { ja: '送信' } },
  'peerbonus.slack.deliveries.attempts': { defaultMessage: '{count} attempts', translations: { ja: '{count} 回試行' } },
  'peerbonus.slack.deliveries.empty': { defaultMessage: 'Nothing sent yet', translations: { ja: 'まだ送信はありません' } },
  'peerbonus.slack.deliveries.emptyBody': {
    defaultMessage: 'Announcements and DMs appear here once someone sends points.',
    translations: { ja: '誰かが pt を贈ると、告知や DM がここに並びます。' },
  },
  'peerbonus.slack.deliveries.emptyFiltered': { defaultMessage: 'No deliveries in this state.', translations: { ja: 'この状態の配送はありません。' } },
})
