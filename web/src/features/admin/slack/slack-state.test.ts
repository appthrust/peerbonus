import { createDashboardTranslator } from '@appthrust/platform-dashboard-i18n'
import { describe, expect, test } from 'bun:test'
import type { SlackAdmin, SlackDeliveryKind, SlackState } from '../../../api/peerbonus'
import { catalogs } from '../../../copy'
import { conversationBlocker, DELIVERY_KIND_LABELS, DELIVERY_STATE_VIEWS, DELIVERY_STATES, SLACK_ERROR_LABELS, setupSteps, STATE_VIEWS } from './slack-state'

const ja = createDashboardTranslator('ja', catalogs)
const en = createDashboardTranslator('en', catalogs)

const base: Pick<SlackAdmin, 'credential' | 'state' | 'settings' | 'inbound'> = {
  credential: { botToken: false, signingSecret: false },
  state: 'unconfigured',
  settings: {
    announceChannel: null,
    announcePosts: false,
    dmRecipientOnPost: true,
    giveFromSlack: false,
    locale: 'ja',
    giveInConversation: false,
    mentionPointChoices: [10, 30, 50],
    reactionRules: [],
  },
  inbound: null,
}

describe('Slack state → status card', () => {
  test('each server state has its chip, tone and single action', () => {
    const rows = (Object.keys(STATE_VIEWS) as SlackState[]).map((state) => {
      const view = STATE_VIEWS[state]
      return [state, ja.t(view.label), en.t(view.label), view.tone, view.action]
    })
    expect(rows).toEqual([
      ['unconfigured', '未設定', 'Not set up', 'neutral', 'none'],
      ['configured', '未確認', 'Not verified', 'busy', 'verify'],
      ['connected', '接続中', 'Connected', 'ok', 'reverify'],
      ['team_changed', 'ワークスペースが変わりました', 'Workspace changed', 'bad', 'force'],
    ])
  })

  test('delivery states and kinds have labels; error codes map to explanations', () => {
    expect(DELIVERY_STATES.map((state) => [ja.t(DELIVERY_STATE_VIEWS[state].label), DELIVERY_STATE_VIEWS[state].tone])).toEqual([
      ['送信待ち', 'busy'],
      ['送信済み', 'ok'],
      ['スキップ', 'neutral'],
      ['失敗', 'bad'],
    ])
    const kinds: SlackDeliveryKind[] = ['post_announce', 'post_dm', 'clap_update', 'post_delete']
    expect(kinds.map((kind) => en.t(DELIVERY_KIND_LABELS[kind]))).toEqual(['Announcement', 'DM to recipient', 'Clap count', 'Deletion'])
    expect(ja.t(SLACK_ERROR_LABELS.user_not_found!)).toBe('受け手のメールアドレスの Slack アカウントがありません')
    expect(SLACK_ERROR_LABELS.something_new).toBeUndefined()
  })
})

describe('setup guide progress', () => {
  const done = (slack: typeof base) => setupSteps(slack).map((step) => `${step.id}:${step.done ? 'done' : 'todo'}`)

  test('no token: nothing done; the signing secret step waits for the inbound endpoint', () => {
    expect(done(base)).toEqual(['createApp:todo', 'pasteToken:todo', 'connect:todo'])
  })

  test('the token marks the app steps; connecting needs the workspace and a channel', () => {
    const token = { ...base, credential: { botToken: true, signingSecret: false }, state: 'configured' as const }
    expect(done(token)).toEqual(['createApp:done', 'pasteToken:done', 'connect:todo'])
    expect(done({ ...token, state: 'connected' })).toEqual(['createApp:done', 'pasteToken:done', 'connect:todo'])
    const channel = { ...token, state: 'connected' as const, settings: { ...base.settings, announceChannel: { id: 'C1', name: 'general' } } }
    expect(done(channel)).toEqual(['createApp:done', 'pasteToken:done', 'connect:done'])
    // A changed workspace undoes the connection step.
    expect(done({ ...channel, state: 'team_changed' })).toEqual(['createApp:done', 'pasteToken:done', 'connect:todo'])
  })

  test('Phase B: the fourth step needs the secret and giving from Slack switched on', () => {
    const phaseB = { ...base, credential: { botToken: true, signingSecret: true }, inbound: { url: 'https://in.example/x', ready: true, reason: null } }
    expect(done(phaseB).at(-1)).toBe('signingSecret:todo')
    expect(done({ ...phaseB, settings: { ...base.settings, giveFromSlack: true } }).at(-1)).toBe('signingSecret:done')
  })
})

describe('giving in a conversation: what keeps the switch off', () => {
  const inbound = { url: 'https://in.example/x', ready: true, reason: null }

  test('the signing secret first, then the inbound URL; nothing once both are there', () => {
    const blocker = (slack: Parameters<typeof conversationBlocker>[0]) => {
      const id = conversationBlocker(slack)
      return id && ja.t(id)
    }
    expect(blocker({ credential: { botToken: true, signingSecret: false }, inbound })).toBe('Dashboard の Credentials に Slack signing secret が必要です。')
    expect(blocker({ credential: { botToken: true, signingSecret: true }, inbound: null })).toBe('Slack からのリクエスト URL がまだないため、使えません。')
    // A projected endpoint without a URL yet (or the platform unreachable) is no URL either.
    expect(blocker({ credential: { botToken: true, signingSecret: true }, inbound: { url: null, ready: false, reason: 'platform_unavailable' } })).toBe(
      'Slack からのリクエスト URL がまだないため、使えません。'
    )
    expect(blocker({ credential: { botToken: true, signingSecret: true }, inbound })).toBeNull()
  })

  test('the usage help shows the mention format in both languages', () => {
    expect(ja.t('peerbonus.slack.conversation.helpExample')).toBe('@Peer Bonus @相手 30 メッセージ')
    expect(en.t('peerbonus.slack.conversation.helpExample')).toBe('@Peer Bonus @person 30 message')
    expect(ja.t('peerbonus.slack.conversation.helpInvite')).toContain('/invite @Peer Bonus')
  })
})
