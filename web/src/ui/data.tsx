// Numbers at a glance: a progress ring (the weekly balance), a bar (value and
// week distributions) and a stat card ("今月 / 累計"). Callers print the
// numbers themselves; the shapes are decoration unless a ring gets a label.
import type { ReactNode } from 'react'
import { cn } from './cn'

const ratio = (value: number, max: number) => (max > 0 ? Math.min(1, Math.max(0, value / max)) : 0)

/**
 * Circular progress of `value / max` (an SVG arc on a stone track). The arc
 * animates when the value changes (stroke-dashoffset transition), so the
 * header balance visibly drops after a send. `children` render centred.
 * With `label` the ring is an image with that accessible name; without it,
 * decorative.
 */
export function Ring({
  value,
  max,
  size = 96,
  strokeWidth,
  label,
  className,
  children,
}: {
  value: number
  max: number
  size?: number
  strokeWidth?: number
  label?: string
  className?: string
  children?: ReactNode
}) {
  const stroke = strokeWidth ?? Math.max(3, Math.round(size / 12))
  const radius = (size - stroke) / 2
  const circumference = 2 * Math.PI * radius
  const offset = circumference * (1 - ratio(value, max))
  return (
    <span
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={cn('relative inline-flex shrink-0 items-center justify-center', className)}
      style={{ width: size, height: size }}
    >
      <svg aria-hidden width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="absolute inset-0 -rotate-90">
        <circle cx={size / 2} cy={size / 2} r={radius} fill="none" strokeWidth={stroke} className="stroke-stone-200" />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          className="stroke-accent transition-[stroke-dashoffset] duration-700 ease-out"
        />
      </svg>
      {children ? <span className="relative flex flex-col items-center justify-center text-center leading-tight">{children}</span> : null}
    </span>
  )
}

/**
 * One bar of a distribution: a rounded track with a fill of `value / max`.
 * Horizontal fills the width; vertical fills the height of its parent (give
 * the parent a fixed height). Always decorative: print the number next to it.
 */
export function Bar({
  value,
  max,
  tone = 'accent',
  orientation = 'horizontal',
  className,
}: {
  value: number
  max: number
  tone?: 'accent' | 'muted'
  orientation?: 'horizontal' | 'vertical'
  className?: string
}) {
  const percent = `${ratio(value, max) * 100}%`
  const fill = tone === 'accent' ? 'bg-accent' : 'bg-stone-400'
  if (orientation === 'vertical') {
    return (
      <span aria-hidden className={cn('flex h-full w-full items-end overflow-hidden rounded-md bg-stone-100', className)}>
        <span className={cn('block w-full rounded-md transition-[height] duration-500 ease-out', fill)} style={{ height: percent }} />
      </span>
    )
  }
  return (
    <span aria-hidden className={cn('block h-2 w-full overflow-hidden rounded-full bg-stone-100', className)}>
      <span className={cn('block h-full rounded-full transition-[width] duration-500 ease-out', fill)} style={{ width: percent }} />
    </span>
  )
}

/**
 * A labelled number ("今月もらった 120 pt"). Renders `<div><dt/><dd/></div>`:
 * wrap cards in a `<dl className="grid …">`.
 */
export function StatCard({ label, value, unit, note, className }: { label: ReactNode; value: ReactNode; unit?: string; note?: ReactNode; className?: string }) {
  return (
    <div className={cn('min-w-0 rounded-lg border border-line bg-white px-4 py-3', className)}>
      <dt className="truncate text-xs font-medium text-stone-500">{label}</dt>
      <dd className="mt-1">
        <span className="text-2xl font-semibold tracking-tight text-stone-900 tabular-nums">{value}</span>
        {unit ? <span className="ml-1 text-sm font-medium text-stone-500">{unit}</span> : null}
        {note ? <span className="mt-0.5 block text-xs text-stone-500">{note}</span> : null}
      </dd>
    </div>
  )
}
