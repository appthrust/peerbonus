// Pill-shaped tags and toggles. Values get a stable hue from their position in
// the Program (so "挑戦" is the same colour on the form, the card, the filter
// bar and in insights); a key the Program no longer defines stays neutral.
import type { ReactNode } from 'react'
import { cn } from './cn'
import { HrefLink } from './link'

/** Static class lists (Tailwind needs the full names): soft fill, ink, ring, and the selected fill. */
const HUES = [
  { soft: 'bg-amber-50 text-amber-800 ring-amber-200', dot: 'bg-amber-500', on: 'bg-amber-600 text-white ring-amber-600' },
  { soft: 'bg-emerald-50 text-emerald-800 ring-emerald-200', dot: 'bg-emerald-500', on: 'bg-emerald-600 text-white ring-emerald-600' },
  { soft: 'bg-sky-50 text-sky-800 ring-sky-200', dot: 'bg-sky-500', on: 'bg-sky-600 text-white ring-sky-600' },
  { soft: 'bg-violet-50 text-violet-800 ring-violet-200', dot: 'bg-violet-500', on: 'bg-violet-600 text-white ring-violet-600' },
  { soft: 'bg-rose-50 text-rose-800 ring-rose-200', dot: 'bg-rose-500', on: 'bg-rose-600 text-white ring-rose-600' },
  { soft: 'bg-teal-50 text-teal-800 ring-teal-200', dot: 'bg-teal-500', on: 'bg-teal-600 text-white ring-teal-600' },
] as const
const NEUTRAL = { soft: 'bg-stone-100 text-stone-700 ring-stone-200', dot: 'bg-stone-400', on: 'bg-stone-800 text-white ring-stone-800' } as const

/** The hue of `key` among the Program's values (its index), or null for a key the Program no longer defines. */
export function valueHue(values: readonly { key: string }[], key: string): number | null {
  const index = values.findIndex((value) => value.key === key)
  return index === -1 ? null : index
}

/** Background class of a hue's solid swatch (bars and dots in charts). */
export function hueDot(hue: number | null): string {
  return hue === null ? NEUTRAL.dot : HUES[hue % HUES.length].dot
}

export type ChipProps = {
  children: ReactNode
  /** Value hue (`valueHue`); `undefined` is the plain neutral chip used for non-value filters. */
  hue?: number | null
  selected?: boolean
  size?: 'sm' | 'md'
  /** A router link (filters, value tags); otherwise a toggle button when `onClick` is set, else a static tag. */
  href?: string
  onClick?: () => void
  /** Toggle buttons only: refuses presses (e.g. a fourth value). */
  disabled?: boolean
  title?: string
  className?: string
}

export function Chip({ children, hue, selected = false, size = 'sm', href, onClick, disabled = false, title, className }: ChipProps) {
  const palette = hue === undefined || hue === null ? NEUTRAL : HUES[hue % HUES.length]
  const classes = cn(
    'relative z-[1] inline-flex max-w-full items-center gap-1 rounded-full font-medium whitespace-nowrap ring-1 ring-inset transition-colors disabled:cursor-not-allowed disabled:opacity-50',
    size === 'md' ? 'min-h-10 px-3.5 text-sm md:min-h-8' : 'min-h-8 px-2.5 text-xs md:min-h-6',
    selected ? palette.on : palette.soft,
    (href || onClick) && !disabled ? (selected ? 'hover:opacity-90' : 'hover:brightness-95') : undefined,
    className
  )
  if (href) {
    return (
      <HrefLink href={href} title={title} aria-current={selected ? 'true' : undefined} className={classes}>
        <span className="truncate">{children}</span>
      </HrefLink>
    )
  }
  if (onClick) {
    return (
      <button type="button" onClick={onClick} disabled={disabled} title={title} aria-pressed={selected} className={classes}>
        <span className="truncate">{children}</span>
      </button>
    )
  }
  return (
    <span title={title} className={classes}>
      <span className="truncate">{children}</span>
    </span>
  )
}
