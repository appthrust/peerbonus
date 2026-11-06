import { useId } from 'react'
import type { StatementSummary } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { cn, controlClasses } from '../../ui'
import { formatMonth } from './format'

/** Every month with points (the statements list), plus this month and the chosen one (a typed-in URL may name another), newest first. */
export function monthOptions(month: string, currentMonth: string, statements: readonly StatementSummary[]): string[] {
  return [...new Set([currentMonth, month, ...statements.map((item) => item.month)])].sort().reverse()
}

/** The month switch of 参加状況 and インサイト; the choice is the page's `?month=`. */
export function MonthPicker({ month, months, onChange }: { month: string; months: readonly string[]; onChange: (month: string) => void }) {
  const { t, locale } = useText()
  const id = useId()
  return (
    <div className="flex items-center gap-2">
      <label htmlFor={id} className="text-sm font-medium text-stone-700">
        {t('peerbonus.admin.col.month')}
      </label>
      <select id={id} value={month} onChange={(event) => onChange(event.target.value)} className={cn(controlClasses, 'h-10 pr-8 md:h-9')}>
        {months.map((option) => (
          <option key={option} value={option}>
            {formatMonth(option, locale)}
          </option>
        ))}
      </select>
    </div>
  )
}
