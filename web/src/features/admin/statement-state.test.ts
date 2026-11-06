import { describe, expect, test } from 'bun:test'
import type { Statement } from '../../api/peerbonus'
import { closeState, nextMonth } from './statement-state'

const statement = (month: string, change: Partial<Pick<Statement, 'closed' | 'closeBlockers' | 'previousOpenMonth'>> = {}) => ({
  month,
  closed: false,
  closeBlockers: [] as Statement['closeBlockers'],
  previousOpenMonth: null,
  ...change,
})

describe('statement close checklist', () => {
  test('a closed month has no checklist', () => {
    expect(closeState(statement('2026-08', { closed: true }))).toEqual({ kind: 'closed' })
  })

  test('no blockers: both checks pass and the month can be closed', () => {
    expect(closeState(statement('2026-09'))).toEqual({
      kind: 'open',
      closable: true,
      checks: [
        { id: 'monthEnded', ok: true, from: '2026-10-01' },
        { id: 'previousClosed', ok: true, month: null },
      ],
    })
  })

  test('the current month waits for the first day of the next month', () => {
    const state = closeState(statement('2026-12', { closeBlockers: ['month_not_ended'] }))
    expect(state).toEqual({
      kind: 'open',
      closable: false,
      checks: [
        { id: 'monthEnded', ok: false, from: '2027-01-01' },
        { id: 'previousClosed', ok: true, month: null },
      ],
    })
  })

  test('an open earlier month names that month; both blockers can hold at once', () => {
    const state = closeState(statement('2026-10', { closeBlockers: ['month_not_ended', 'previous_month_open'], previousOpenMonth: '2026-08' }))
    expect(state.kind === 'open' && state.closable).toBe(false)
    expect(state.kind === 'open' ? state.checks : []).toEqual([
      { id: 'monthEnded', ok: false, from: '2026-11-01' },
      { id: 'previousClosed', ok: false, month: '2026-08' },
    ])
  })

  test('nextMonth rolls the year over', () => {
    expect(nextMonth('2026-09')).toBe('2026-10')
    expect(nextMonth('2026-12')).toBe('2027-01')
  })
})
