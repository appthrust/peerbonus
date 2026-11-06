import { Check } from 'lucide-react'
import { useState } from 'react'
import type { Value } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { MAX_VALUES } from '../../model/post'
import { Chip, valueHue } from '../../ui'

/** Values shown before 「もっと」. */
const VISIBLE_VALUES = 3

/**
 * The Program's values as toggle chips in their value colours: the first three, any chosen one,
 * and the rest behind 「もっと」. Once `MAX_VALUES` are chosen the others are disabled.
 */
export function ValueChips({
  labelledBy,
  describedBy,
  values,
  selected,
  onChange,
}: {
  labelledBy: string
  describedBy?: string
  values: readonly Value[]
  selected: readonly string[]
  onChange: (next: string[]) => void
}) {
  const { t } = useText()
  const [expanded, setExpanded] = useState(false)
  const full = selected.length >= MAX_VALUES
  const shown = expanded ? values : values.filter((value, index) => index < VISIBLE_VALUES || selected.includes(value.key))
  const hidden = values.length - shown.length
  return (
    <div role="group" aria-labelledby={labelledBy} aria-describedby={describedBy} className="flex flex-wrap items-center gap-1.5">
      {shown.map((value) => {
        const on = selected.includes(value.key)
        return (
          <Chip
            key={value.key}
            size="md"
            hue={valueHue(values, value.key)}
            selected={on}
            disabled={!on && full}
            title={value.description ?? undefined}
            onClick={() => onChange(on ? selected.filter((key) => key !== value.key) : [...selected, value.key])}
          >
            {on ? <Check aria-hidden className="mr-0.5 -mt-0.5 inline size-3.5" /> : null}#{value.label}
          </Chip>
        )
      })}
      {hidden > 0 || expanded ? (
        <button
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded(!expanded)}
          className="inline-flex min-h-10 items-center rounded-full px-2.5 text-xs font-medium text-accent-strong hover:underline focus-visible:outline-2 focus-visible:outline-accent md:min-h-8"
        >
          {expanded ? t('peerbonus.composer.valuesLess') : t('peerbonus.composer.valuesMore', { count: hidden })}
        </button>
      ) : null}
    </div>
  )
}
