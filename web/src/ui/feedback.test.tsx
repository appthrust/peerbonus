import { describe, expect, test } from 'bun:test'
import type { ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { PeerbonusApiError } from '../api/peerbonus'
import { I18nProvider } from '../i18n'
import { ActionError, LoadFailed } from './feedback'

const render = (node: ReactNode, language: 'ja' | 'en' = 'ja') => renderToStaticMarkup(<I18nProvider language={language}>{node}</I18nProvider>)

describe('peerbonus error feedback', () => {
  test('insufficient_points names the balance and the cost', () => {
    const error = new PeerbonusApiError(409, { error: 'insufficient_points', message: 'Not enough points remain this week', remaining: 4, required: 30 })
    const html = render(<ActionError error={error} />)
    expect(html).toContain('role="alert"')
    expect(html).toContain('今週の残りポイントが足りません（残り 4 pt、必要 30 pt）。')
    expect(render(<ActionError error={error} />, 'en')).toContain('4 pt left, 30 pt needed.')
  })

  test('own_post, self_target and directory_unavailable explain themselves instead of a generic failure', () => {
    expect(render(<ActionError error={new PeerbonusApiError(403, { error: 'own_post', message: '' })} />)).toContain('自分が贈った・もらった投稿には拍手できません。')
    expect(render(<ActionError error={new PeerbonusApiError(400, { error: 'self_target', message: '' })} />)).toContain('自分にはポイントを贈れません。')
    expect(render(<ActionError error={new PeerbonusApiError(503, { error: 'directory_unavailable', message: '' })} />)).toContain('Directory に一時的につながらない')
  })

  test('an invalid_request keeps the server detail under the localized summary; unknown errors stay generic', () => {
    const invalid = render(<ActionError error={new PeerbonusApiError(400, { error: 'invalid_request', message: 'points must be an integer between 1 and 120' })} />)
    expect(invalid).toContain('入力内容を確認してください。')
    expect(invalid).toContain('points must be an integer between 1 and 120')
    expect(render(<ActionError error={new Error('boom')} />)).toContain('操作を完了できませんでした')
  })

  test('a 401 load failure offers no retry; a store outage explains itself and offers one', () => {
    const denied = render(<LoadFailed error={new PeerbonusApiError(401, { error: 'unauthenticated', message: '' })} onRetry={() => {}} />)
    expect(denied).toContain('このアカウントではピアボーナスを使えません')
    expect(denied).not.toContain('<button')
    const store = render(<LoadFailed error={new PeerbonusApiError(503, { error: 'peerbonus_store_unavailable', message: 'database unreachable' })} onRetry={() => {}} />)
    expect(store).toContain('もう一度読み込む')
    expect(store).toContain('データベースに一時的につながりません')
  })
})
