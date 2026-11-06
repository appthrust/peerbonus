import { Minus, Plus } from 'lucide-react'
import { useText } from '../../i18n'
import { clampPoints } from '../../model/post'
import { cn, controlClasses, IconButton } from '../../ui'

const PRESETS = [10, 30, 50, 100] as const

/**
 * Points stepper: a numeric spin button (arrow keys step by 1) between − / +
 * buttons, plus quick presets up to `max`. `value` is NaN while the field is
 * empty; the form reports it as out of range.
 */
export function PointInput({
  id,
  value,
  max,
  invalid,
  onChange,
}: {
  id: string
  value: number
  max: number
  invalid?: boolean
  onChange: (points: number) => void
}) {
  const { t } = useText()
  const presets = PRESETS.filter((preset) => preset <= max)
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="flex items-center gap-1">
        <IconButton
          size="md"
          tone="secondary"
          icon={<Minus />}
          label={t('peerbonus.compose.pointsDecrease')}
          aria-controls={id}
          disabled={Number.isFinite(value) && value <= 1}
          onClick={() => onChange(clampPoints((Number.isFinite(value) ? value : 2) - 1, max))}
        />
        <input
          id={id}
          type="number"
          inputMode="numeric"
          min={1}
          max={max}
          step={1}
          value={Number.isNaN(value) ? '' : value}
          onChange={(event) => onChange(event.target.value === '' ? Number.NaN : Number(event.target.value))}
          aria-invalid={invalid || undefined}
          aria-describedby={`${id}-${invalid ? 'error' : 'hint'}`}
          className={cn(controlClasses, 'h-10 w-20 text-center text-base font-semibold tabular-nums md:h-9')}
        />
        <IconButton
          size="md"
          tone="secondary"
          icon={<Plus />}
          label={t('peerbonus.compose.pointsIncrease')}
          aria-controls={id}
          disabled={Number.isFinite(value) && value >= max}
          onClick={() => onChange(clampPoints((Number.isFinite(value) ? value : 0) + 1, max))}
        />
      </div>
      {presets.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {presets.map((preset) => (
            <button
              key={preset}
              type="button"
              aria-pressed={value === preset}
              aria-label={t('peerbonus.compose.pointsPreset', { points: preset })}
              onClick={() => onChange(preset)}
              className={cn(
                'min-h-10 rounded-full border px-3 text-xs font-medium tabular-nums md:min-h-8',
                value === preset ? 'border-accent bg-accent-soft text-accent-strong' : 'border-stone-300 bg-white text-stone-600 hover:border-stone-400 hover:text-stone-900'
              )}
            >
              {t('peerbonus.common.points', { points: preset })}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
