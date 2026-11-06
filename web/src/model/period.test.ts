import { describe, expect, test } from 'bun:test'
import { monthEnded, monthKeyAt, periodAt, previousMonth, timeUntil } from './period'

describe('Program-time-zone weeks and months', () => {
  test('a week runs Monday 00:00 to Monday 00:00 in the zone and carries its ISO week key', () => {
    // Monday 2026-10-19 00:30 in Tokyo is still Sunday in UTC.
    expect(periodAt(Date.parse('2026-10-18T15:30:00Z'), 'Asia/Tokyo')).toEqual({ key: '2026-W43', startsAt: '2026-10-18T15:00:00.000Z', endsAt: '2026-10-25T15:00:00.000Z' })
    expect(periodAt(Date.parse('2026-10-18T14:59:59Z'), 'Asia/Tokyo').key).toBe('2026-W42')
    // ISO years: 2027-01-01 (Friday) belongs to 2026-W53; 2025-12-29 (Monday) to 2026-W01.
    expect(periodAt(Date.parse('2027-01-01T03:00:00Z'), 'Asia/Tokyo').key).toBe('2026-W53')
    expect(periodAt(Date.parse('2025-12-29T03:00:00Z'), 'Asia/Tokyo').key).toBe('2026-W01')
    // A DST change inside the week shortens it by an hour (America/New_York, 2026-11-01).
    const week = periodAt(Date.parse('2026-10-29T12:00:00Z'), 'America/New_York')
    expect(week).toEqual({ key: '2026-W44', startsAt: '2026-10-26T04:00:00.000Z', endsAt: '2026-11-02T05:00:00.000Z' })
  })

  test('months, month ends and the previous month', () => {
    expect(monthKeyAt(Date.parse('2026-10-31T15:00:00Z'), 'Asia/Tokyo')).toBe('2026-11')
    expect(monthKeyAt(Date.parse('2026-10-31T15:00:00Z'), 'UTC')).toBe('2026-10')
    expect(monthEnded('2026-10', Date.parse('2026-10-31T14:59:59Z'), 'Asia/Tokyo')).toBe(false)
    expect(monthEnded('2026-10', Date.parse('2026-10-31T15:00:00Z'), 'Asia/Tokyo')).toBe(true)
    expect(monthEnded('2026-12', Date.parse('2026-12-31T15:00:00Z'), 'Asia/Tokyo')).toBe(true)
    expect(previousMonth('2026-01')).toBe('2025-12')
    expect(previousMonth('2026-10')).toBe('2026-09')
  })

  test('the reset countdown: whole days, then hours', () => {
    const now = Date.parse('2026-10-21T03:00:00Z')
    expect(timeUntil('2026-10-25T15:00:00.000Z', now)).toEqual({ days: 5, hours: 0 })
    expect(timeUntil('2026-10-21T08:30:00.000Z', now)).toEqual({ days: 0, hours: 6 })
    expect(timeUntil('2026-10-21T02:00:00.000Z', now)).toEqual({ days: 0, hours: 1 })
  })
})
