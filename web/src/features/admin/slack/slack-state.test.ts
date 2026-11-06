import { createDashboardTranslator } from '@appthrust/platform-dashboard-i18n'
import { describe, expect, test } from 'bun:test'
import type { SlackAdmin, SlackDeliveryKind, SlackState } from '../../../api/peerbonus'
import { catalogs } from '../../../copy'
import { DELIVERY_KIND_LABELS, DELIVERY_STATE_VIEWS, DELIVERY_STATES, SLACK_ERROR_LABELS, setupSteps, STATE_VIEWS } from './slack-state'

const ja = createDashboardTranslator('ja', catalogs)
const en = createDashboardTranslator('en', catalogs)

const base: Pick<SlackAdmin, 'credential' | 'state' | 'settings' | 'inbound'> = {
  credential: { botToken: false, signingSecret: false },
  state: 'unconfigured',
  settings: { announceChannel: null, announcePosts: false, dmRecipientOnPost: true, giveFromSlack: false, locale: 'ja' },
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
