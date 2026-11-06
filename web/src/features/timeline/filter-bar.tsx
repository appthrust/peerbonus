import { Search, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useText } from '../../i18n'
import { monthKeyAt, previousMonth } from '../../model/period'
import { paths } from '../../routes/paths'
import { useSession } from '../../shell/use-session'
import { Chip, cn, controlClasses, Tabs, valueHue } from '../../ui'
import { valueLabel } from '../posts/value-tag'

/** The timeline's URL state (`?value=&month=&q=&mine=1`). */
export type TimelineFilters = { value?: string; month?: string; q?: string; mine?: boolean }

/** How long typing pauses before the search goes to the URL. */
const SEARCH_DEBOUNCE_MS = 300
/** The month select offers the current month and this many before it. */
const PAST_MONTHS = 5

/**
 * すべて / 自分, every value as a chip (a key only old posts carry stays
 * selectable), a month (this one and five before) and a text search
 * (debounced; Enter searches at once). Chips and the segment are links, so
 * every filter is a URL; the month and the search go through `onChange`.
 */
export function FilterBar({
  filters,
  now,
  onChange,
}: {
  filters: TimelineFilters
  /** The page's clock; the month list starts at its month. */
  now: number
  onChange: (next: TimelineFilters, options?: { replace?: boolean }) => void
}) {
  const { t, locale } = useText()
  const { me } = useSession()
  const values = me.program.values
  const keys = values.map((item) => item.key)
  if (filters.value && !keys.includes(filters.value)) keys.push(filters.value)

  const months = [monthKeyAt(now, me.program.timeZone)]
  while (months.length <= PAST_MONTHS) months.push(previousMonth(months[months.length - 1]))
  if (filters.month && !months.includes(filters.month)) months.push(filters.month)
  const monthName = new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'long', timeZone: 'UTC' })

  // The box follows the URL (a cleared filter, back/forward) unless it only differs by spaces still being typed.
  const [text, setText] = useState(filters.q ?? '')
  const [shown, setShown] = useState(filters.q)
  if (filters.q !== shown) {
    setShown(filters.q)
    if ((filters.q ?? '') !== text.trim()) setText(filters.q ?? '')
  }
  const submit = (next: string) => {
    const q = next.trim() || undefined
    if (q !== filters.q) onChange({ ...filters, q }, { replace: true })
  }
  // Every keystroke re-renders and restarts the pause; nothing is pending once the URL holds the text.
  useEffect(() => {
    if ((text.trim() || undefined) === filters.q) return
    const timer = setTimeout(() => submit(text), SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(timer)
  })

  return (
    <div role="search" aria-label={t('peerbonus.timeline.filterBar')} className="grid gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Tabs
          variant="segmented"
          label={t('peerbonus.timeline.scope')}
          items={[
            { href: paths.timeline({ ...filters, mine: false }), label: t('peerbonus.timeline.scopeAll') },
            { href: paths.timeline({ ...filters, mine: true }), label: t('peerbonus.timeline.scopeMine') },
          ]}
        />
        <select
          aria-label={t('peerbonus.timeline.month')}
          value={filters.month ?? ''}
          onChange={(event) => onChange({ ...filters, month: event.target.value || undefined })}
          className={cn(controlClasses, 'h-10 w-auto pr-8 md:h-9')}
        >
          <option value="">{t('peerbonus.timeline.monthAll')}</option>
          {months.map((month) => (
            <option key={month} value={month}>
              {monthName.format(Date.UTC(Number(month.slice(0, 4)), Number(month.slice(5, 7)) - 1, 1))}
            </option>
          ))}
        </select>
        <form
          className="relative min-w-0 basis-full sm:flex-1 sm:basis-56"
          onSubmit={(event) => {
            event.preventDefault()
            submit(text)
          }}
        >
          <Search aria-hidden className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-stone-400" />
          <input
            type="search"
            enterKeyHint="search"
            maxLength={100}
            aria-label={t('peerbonus.timeline.search')}
            placeholder={t('peerbonus.timeline.searchPlaceholder')}
            value={text}
            onChange={(event) => setText(event.target.value)}
            className={cn(controlClasses, 'h-10 w-full pr-10 pl-9 md:h-9 [&::-webkit-search-cancel-button]:hidden')}
          />
          {text ? (
            <button
              type="button"
              aria-label={t('peerbonus.timeline.searchClear')}
              title={t('peerbonus.timeline.searchClear')}
              onClick={() => {
                setText('')
                submit('')
              }}
              className="absolute top-1/2 right-1 flex size-8 -translate-y-1/2 items-center justify-center rounded-md text-stone-400 hover:bg-stone-100 hover:text-stone-700 md:size-7"
            >
              <X aria-hidden className="size-4" />
            </button>
          ) : null}
        </form>
      </div>
      {keys.length > 0 ? (
        <nav aria-label={t('peerbonus.timeline.filters')}>
          <ul className="flex flex-wrap gap-1.5">
            {keys.map((key) => (
              <li key={key} className="flex">
                <Chip
                  href={paths.timeline({ ...filters, value: filters.value === key ? undefined : key })}
                  hue={valueHue(values, key)}
                  selected={filters.value === key}
                  title={values.find((item) => item.key === key)?.description ?? undefined}
                >
                  {valueLabel(values, key)}
                </Chip>
              </li>
            ))}
          </ul>
        </nav>
      ) : null}
    </div>
  )
}
