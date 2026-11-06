// Whether a month can be closed right now, as the server says it
// (`GET /admin/statements/{month}` → `closeBlockers`, `previousOpenMonth`):
// only after it ended in the program time zone, and oldest first. The detail
// page turns this into the close checklist and enables 締める only when every
// check passes; the server still enforces both (409 `month_not_ended` /
// `previous_month_open`).
import type { Statement } from '../../api/peerbonus'

/** One line of the close checklist. */
export type CloseCheck =
  /** `from`: the first day of the next month ("YYYY-MM-01"), 00:00 in the program time zone. */
  | { id: 'monthEnded'; ok: boolean; from: string }
  /** `month`: the oldest earlier month that is still open (null once none is). */
  | { id: 'previousClosed'; ok: boolean; month: string | null }

export type CloseState = { kind: 'closed' } | { kind: 'open'; closable: boolean; checks: CloseCheck[] }

/** "2026-12" → "2027-01". */
export function nextMonth(month: string): string {
  const [year, mon] = month.split('-').map(Number) as [number, number]
  return mon === 12 ? `${year + 1}-01` : `${year}-${String(mon + 1).padStart(2, '0')}`
}

/** The checklist for one statement, derived from the server's blockers; closable when nothing blocks. */
export function closeState(statement: Pick<Statement, 'month' | 'closed' | 'closeBlockers' | 'previousOpenMonth'>): CloseState {
  if (statement.closed) return { kind: 'closed' }
  const blocked = (blocker: Statement['closeBlockers'][number]) => statement.closeBlockers.includes(blocker)
  const checks: CloseCheck[] = [
    { id: 'monthEnded', ok: !blocked('month_not_ended'), from: `${nextMonth(statement.month)}-01` },
    { id: 'previousClosed', ok: !blocked('previous_month_open'), month: blocked('previous_month_open') ? statement.previousOpenMonth : null },
  ]
  return { kind: 'open', closable: statement.closeBlockers.length === 0, checks }
}
