// Week and month keys in the Program time zone (domain-model.md §8): a week
// runs Monday 00:00 to the next Monday 00:00 and is keyed by its ISO week
// (`2026-W43`); a month is the calendar month (`2026-10`). Shared by the dev
// mock (which assigns keys like the server) and the screens (the reset time).

const DAY = 86_400_000

const formatters = new Map<string, Intl.DateTimeFormat>()

function formatter(timeZone: string): Intl.DateTimeFormat {
  let format = formatters.get(timeZone)
  if (!format) {
    format = new Intl.DateTimeFormat('en-US', {
      timeZone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
    formatters.set(timeZone, format)
  }
  return format
}

type Wall = { year: number; month: number; day: number; hour: number; minute: number; second: number }

/** The wall-clock reading of `ms` in `timeZone`. */
function wallClock(ms: number, timeZone: string): Wall {
  const parts: Record<string, number> = {}
  for (const part of formatter(timeZone).formatToParts(new Date(ms))) {
    if (part.type !== 'literal') parts[part.type] = Number(part.value)
  }
  return { year: parts.year!, month: parts.month!, day: parts.day!, hour: parts.hour!, minute: parts.minute!, second: parts.second! }
}

/** How far `timeZone` is ahead of UTC at `ms`, in milliseconds. */
function offsetAt(ms: number, timeZone: string): number {
  const wall = wallClock(ms, timeZone)
  return Date.UTC(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute, wall.second) - Math.floor(ms / 1000) * 1000
}

/** The instant the wall clock in `timeZone` reads midnight of the given date (month is 1-based and may overflow). */
function midnight(year: number, month: number, day: number, timeZone: string): number {
  const guess = Date.UTC(year, month - 1, day)
  const first = guess - offsetAt(guess, timeZone)
  // A DST change between the guess and the answer shifts the offset once.
  return guess - offsetAt(first, timeZone)
}

const pad = (value: number, width = 2) => String(value).padStart(width, '0')

export type PeriodBounds = { key: string; startsAt: string; endsAt: string }

/** The ISO week containing `ms` in `timeZone`, with its Monday-to-Monday bounds as ISO timestamps. */
export function periodAt(ms: number, timeZone: string): PeriodBounds {
  const wall = wallClock(ms, timeZone)
  const date = Date.UTC(wall.year, wall.month - 1, wall.day)
  const sinceMonday = (new Date(date).getUTCDay() + 6) % 7
  const thursday = new Date(date + (3 - sinceMonday) * DAY)
  const isoYear = thursday.getUTCFullYear()
  const week = Math.floor((thursday.getTime() - Date.UTC(isoYear, 0, 1)) / (7 * DAY)) + 1
  const start = midnight(wall.year, wall.month, wall.day - sinceMonday, timeZone)
  const end = midnight(wall.year, wall.month, wall.day - sinceMonday + 7, timeZone)
  return { key: `${isoYear}-W${pad(week)}`, startsAt: new Date(start).toISOString(), endsAt: new Date(end).toISOString() }
}

/** `YYYY-MM` of `ms` in `timeZone`. */
export function monthKeyAt(ms: number, timeZone: string): string {
  const wall = wallClock(ms, timeZone)
  return `${wall.year}-${pad(wall.month)}`
}

const MONTH_KEY = /^(\d{4})-(0[1-9]|1[0-2])$/

export function isMonthKey(value: string): boolean {
  return MONTH_KEY.test(value)
}

/** The month a close is allowed for: `ms` is at or after the next month's first midnight in `timeZone`. */
export function monthEnded(month: string, ms: number, timeZone: string): boolean {
  const match = MONTH_KEY.exec(month)
  if (!match) return false
  return ms >= midnight(Number(match[1]), Number(match[2]) + 1, 1, timeZone)
}

/** `2026-01` → `2025-12`. */
export function previousMonth(month: string): string {
  const match = MONTH_KEY.exec(month)
  if (!match) throw new Error(`not a month key: ${month}`)
  const year = Number(match[1])
  const index = Number(match[2]) - 1
  return index === 0 ? `${year - 1}-12` : `${year}-${pad(index)}`
}

/** Whole days until `endsAt` (rounded up, never negative) and the remaining hours when less than a day is left. */
export function timeUntil(endsAt: string, now: number): { days: number; hours: number } {
  const left = Math.max(0, Date.parse(endsAt) - now)
  if (left >= DAY) return { days: Math.ceil(left / DAY), hours: 0 }
  return { days: 0, hours: Math.max(1, Math.ceil(left / 3_600_000)) }
}
