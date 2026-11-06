// Display formatting for the admin tables. Amounts arrive as decimal strings
// ("1234.00") so JSON never rounds them; they are only formatted here.

/** "2026-10" → "2026年10月" / "October 2026". */
export function formatMonth(month: string, locale: string): string {
  const [year, mon] = month.split('-').map(Number) as [number, number]
  return new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'long', timeZone: 'UTC' }).format(Date.UTC(year, mon - 1, 1))
}

/** "2026-10" → "10月" / "Oct" (the year when it is January, so a chart crossing years stays readable). */
export function formatMonthShort(month: string, locale: string): string {
  const [year, mon] = month.split('-').map(Number) as [number, number]
  return new Intl.DateTimeFormat(locale, { ...(mon === 1 ? { year: '2-digit' } : {}), month: 'short', timeZone: 'UTC' }).format(Date.UTC(year, mon - 1, 1))
}

/** "1234.5" → "1,234.5": grouped, keeping the server's fraction digits. */
export function formatDecimal(amount: string, locale: string): string {
  const fraction = amount.split('.')[1]?.length ?? 0
  return new Intl.NumberFormat(locale, { minimumFractionDigits: fraction, maximumFractionDigits: fraction }).format(Number(amount))
}

/** "1234.5" + "JPY" → "￥1,234.5" / "¥1,234.5"; keeps the server's fraction digits. An unknown currency code falls back to "1,234.5 XYZ". */
export function formatMoney(amount: string, currency: string, locale: string): string {
  const fraction = amount.split('.')[1]?.length ?? 0
  try {
    return new Intl.NumberFormat(locale, { style: 'currency', currency, minimumFractionDigits: fraction, maximumFractionDigits: fraction }).format(Number(amount))
  } catch {
    return `${formatDecimal(amount, locale)} ${currency}`
  }
}

/** Grouped integer ("1,234"). */
export function formatCount(value: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(value)
}

/** "2026-11-01" as a calendar date, independent of the reader's zone. */
export function formatDay(day: string, locale: string): string {
  const [year, mon, date] = day.split('-').map(Number) as [number, number, number]
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(Date.UTC(year, mon - 1, date))
}
