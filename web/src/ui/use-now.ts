import { useEffect, useState } from 'react'

/** How often a page's clock moves: relative times are minute-grained. */
export const CLOCK_TICK_MS = 60_000

/**
 * A clock for relative times and date checks that keeps render pure (no
 * `Date.now()` per render). With `refreshMs` the value moves on that interval,
 * so "3 時間前", a week-end reminder or the current month stay true on a page
 * left open; a page owns one ticking clock and passes it down. Without it the
 * value is the time of the first render (a per-card fallback, no timer each).
 */
export function useNow(refreshMs?: number): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!refreshMs) return
    const timer = setInterval(() => setNow(Date.now()), refreshMs)
    return () => clearInterval(timer)
  }, [refreshMs])
  return now
}
