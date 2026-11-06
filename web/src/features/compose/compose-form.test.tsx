import { beforeEach, describe, expect, test } from 'bun:test'
import { peerbonusRoutes } from '../../dev/mock-peerbonus'
import { clampPoints, composeProblems, messageProblem } from '../../model/post'
import { mockMe, render } from '../../test/render'
import { ComposeForm } from './compose-form'
import { ComposeLauncher } from './launcher'

beforeEach(async () => {
  const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/reset')
  await peerbonusRoutes(new Request(url, { method: 'POST' }), url)
})

const draft = { recipientPersonId: 'p-sato', points: 30, message: 'ありがとう', values: ['teamwork'] }
const limits = { max: 120, remaining: 400 }

describe('compose validation', () => {
  test('a complete draft within the limits has no problems', () => {
    expect(composeProblems(draft, limits)).toEqual({})
  })

  test('recipient, points range, balance, message and values each report their problem', () => {
    expect(composeProblems({ ...draft, recipientPersonId: null }, limits)).toEqual({ recipient: 'recipientRequired' })
    expect(composeProblems({ ...draft, points: 0 }, limits)).toEqual({ points: 'pointsRange' })
    expect(composeProblems({ ...draft, points: 121 }, limits)).toEqual({ points: 'pointsRange' })
    expect(composeProblems({ ...draft, points: Number.NaN }, limits)).toEqual({ points: 'pointsRange' })
    expect(composeProblems({ ...draft, points: 2.5 }, limits)).toEqual({ points: 'pointsRange' })
    expect(composeProblems({ ...draft, points: 50 }, { max: 120, remaining: 40 })).toEqual({ points: 'pointsInsufficient' })
    expect(composeProblems({ ...draft, values: ['a', 'b', 'c', 'd'] }, limits)).toEqual({ values: 'valuesTooMany' })
    expect(composeProblems({ ...draft, values: ['a', 'b', 'c'] }, limits)).toEqual({})
  })

  test('the message is trimmed and counted in code points, 1..500, without control characters', () => {
    expect(messageProblem('   \n ')).toBe('messageRequired')
    expect(messageProblem('😀'.repeat(500))).toBeUndefined()
    expect(messageProblem(` ${'あ'.repeat(501)} `)).toBe('messageTooLong')
    expect(messageProblem('行1\n\t行2')).toBeUndefined()
    expect(messageProblem('bell\u0007')).toBe('messageControl')
  })

  test('the stepper clamps to 1..max', () => {
    expect(clampPoints(0, 120)).toBe(1)
    expect(clampPoints(500, 120)).toBe(120)
    expect(clampPoints(Number.NaN, 120)).toBe(1)
    expect(clampPoints(5, 0)).toBe(1)
  })
})

describe('compose form', () => {
  test('shows what a send leaves (not the balance), the stepper bounds, the counter and three values; sends 30 pt by default', async () => {
    const me = await mockMe()
    const html = render(<ComposeForm />, me)
    const { remaining, allowance } = me.stats.period
    expect(html).toContain(`送ると今週あと ${remaining - 30} pt`)
    // The balance and its reset live in the header only.
    expect(html).not.toContain(`/ ${allowance} pt`)
    expect(html).not.toContain('リセットまであと')
    expect(html).not.toContain('必須')
    expect(html).toContain('任意')
    // The recipient picker is loading its people (static markup never fetches); its search comes first.
    expect(html).toContain('贈れる相手を読み込んでいます…')
    expect(html.indexOf('type="search"')).toBeLessThan(html.indexOf('贈れる相手を読み込んでいます…'))
    expect(html).toContain(`max="${me.period.maxPointsPerPost}"`)
    expect(html).toContain('aria-label="ポイントを減らす"')
    expect(html).toContain('0 / 500')
    // Values: the first three, the rest behind もっと.
    const labels = me.program.values.map((value) => value.label)
    expect(labels.length).toBeGreaterThan(3)
    for (const label of labels.slice(0, 3)) expect(html).toContain(`#${label}`)
    for (const label of labels.slice(3)) expect(html).not.toContain(`#${label}`)
    expect(html).toContain(`もっと（${labels.length - 3}）`)
    expect(html).toContain('>30 pt 贈る<')
    expect(html).not.toContain('今週のポイントを使い切りました')
  })

  test('a chosen value beyond the first three stays visible', async () => {
    const me = await mockMe()
    const last = me.program.values.at(-1)!
    const html = render(<ComposeForm draft={{ recipient: null, points: 30, message: '', values: [last.key] }} />, me)
    expect(html).toContain(`#${last.label}`)
    expect(html).toMatch(new RegExp(`aria-pressed="true"[^>]*>.*#${last.label}`))
  })

  test('a small balance lowers the default; points beyond it say so at once; an empty balance explains itself and disables sending', async () => {
    const me = await mockMe()
    const low = { ...me, stats: { ...me.stats, period: { ...me.stats.period, remaining: 12 } } }
    const lowHtml = render(<ComposeForm />, low)
    expect(lowHtml).toContain('12 pt 贈る')
    expect(lowHtml).toContain('送ると今週あと 0 pt')
    expect(render(<ComposeForm draft={{ recipient: null, points: 30, message: '', values: [] }} />, low)).toContain('今週の残りは 12 pt です。')
    const empty = { ...me, stats: { ...me.stats, period: { ...me.stats.period, remaining: 0 } } }
    const html = render(<ComposeForm />, empty)
    expect(html).toContain('今週のポイントを使い切りました')
    expect(html).toMatch(/<button type="submit"[^>]*disabled=""/)
  })

  test('キャンセル shows only when the form can close', async () => {
    const me = await mockMe()
    expect(render(<ComposeForm />, me)).not.toContain('キャンセル')
    expect(render(<ComposeForm onCancel={() => {}} />, me)).toContain('キャンセル')
  })
})

describe('compose launcher', () => {
  test('closed: one input-like row with the viewer, no form', async () => {
    const me = await mockMe()
    const html = render(<ComposeLauncher />, me)
    expect(html).toContain('だれかに感謝を贈る…')
    expect(me.member.avatarUrl).not.toBeNull()
    expect(html).toContain(`src="${me.member.avatarUrl}"`)
    expect(render(<ComposeLauncher />, { ...me, member: { ...me.member, avatarUrl: null } })).toContain(`>${me.member.displayName.slice(0, 1)}<`)
    expect(html).not.toContain('<form')
    expect(render(<ComposeLauncher />, me, 'en')).toContain('Thank someone…')
  })

  test('open (defaultOpen, open, or ?to=): the form in place with its title and close', async () => {
    const me = await mockMe()
    for (const node of [<ComposeLauncher defaultOpen />, <ComposeLauncher open />, <ComposeLauncher to="p-sato" />]) {
      const html = render(node, me)
      expect(html).toContain('<form')
      expect(html).toContain('感謝を贈る</h2>')
      expect(html).toContain('aria-label="フォームを閉じる"')
      expect(html).toContain('キャンセル')
      expect(html).not.toContain('だれかに感謝を贈る…')
    }
    expect(render(<ComposeLauncher open={false} defaultOpen />, me)).not.toContain('<form')
  })
})
