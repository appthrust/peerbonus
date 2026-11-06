import { useState } from 'react'
import { cn } from './cn'

const AVATAR_SIZE = { sm: 'size-6 text-[10px]', md: 'size-8 text-xs', lg: 'size-16 text-xl' } as const

/**
 * Up to two initials: the first letters of the first two words of a Latin name
 * ("Ada Lovelace" → "AL"), else the first character ("山田 太郎" → "山").
 */
export function initialsOf(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  const first = Array.from(words[0] ?? '')[0]
  if (!first) return '?'
  const second = Array.from(words[1] ?? '')[0]
  return /^[A-Za-z]$/.test(first) && second && /^[A-Za-z]$/.test(second) ? `${first}${second}`.toUpperCase() : first.toUpperCase()
}

/**
 * A member's profile picture (relayed by Peer Bonus from the Directory on this
 * origin), else their initials in a neutral circle — also when the picture
 * fails to load. Decorative; the name is always shown next to it.
 */
export function Avatar({ name, avatarUrl, size = 'md' }: { name: string; avatarUrl?: string | null; size?: keyof typeof AVATAR_SIZE }) {
  const [failedUrl, setFailedUrl] = useState<string>()
  if (avatarUrl && avatarUrl !== failedUrl) {
    return (
      <img
        aria-hidden
        alt=""
        src={avatarUrl}
        loading="lazy"
        decoding="async"
        onError={() => setFailedUrl(avatarUrl)}
        className={cn('inline-block shrink-0 rounded-full bg-stone-200 object-cover', AVATAR_SIZE[size])}
      />
    )
  }
  return (
    <span
      aria-hidden
      className={cn('inline-flex shrink-0 items-center justify-center rounded-full bg-accent-soft font-semibold text-accent-strong ring-1 ring-accent-line ring-inset', AVATAR_SIZE[size])}
    >
      {initialsOf(name)}
    </span>
  )
}

/**
 * Up to `max` overlapping avatars (newest first) and "+N" for the rest of
 * `total`. Decorative: pair it with the names or a count in text.
 */
export function AvatarStack({
  members,
  total = members.length,
  max = 3,
  size = 'sm',
}: {
  members: readonly { displayName: string; avatarUrl: string | null }[]
  total?: number
  max?: number
  size?: 'sm' | 'md'
}) {
  const shown = members.slice(0, max)
  const rest = total - shown.length
  if (shown.length === 0) return null
  return (
    <span aria-hidden className="inline-flex shrink-0 items-center">
      {shown.map((member, index) => (
        <span key={`${member.displayName}-${index}`} className={cn('inline-flex rounded-full ring-2 ring-white', index > 0 ? '-ml-1.5' : undefined)}>
          <Avatar name={member.displayName} avatarUrl={member.avatarUrl} size={size} />
        </span>
      ))}
      {rest > 0 ? (
        <span className={cn('-ml-1.5 inline-flex min-w-6 items-center justify-center rounded-full bg-stone-100 px-1.5 font-semibold text-stone-600 ring-2 ring-white tabular-nums', size === 'sm' ? 'h-6 text-[10px]' : 'h-8 text-xs')}>
          +{rest}
        </span>
      ) : null}
    </span>
  )
}

/** Small neutral label next to a title or name (「未ログイン」, 「あなた」). */
export function Badge({ children, tone = 'neutral' }: { children: string; tone?: 'neutral' | 'accent' }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium whitespace-nowrap ring-1 ring-inset',
        tone === 'accent' ? 'bg-accent-soft text-accent-strong ring-accent-line' : 'bg-stone-100 text-stone-600 ring-stone-200'
      )}
    >
      {children}
    </span>
  )
}

/** "2026年10月16日" / "Oct 16, 2026" for an ISO timestamp, in the reader's time zone. */
export function formatDate(iso: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(new Date(iso))
}

/** "2026/10/16 14:05" / "Oct 16, 2026, 2:05 PM" for an ISO timestamp, in the reader's time zone. */
export function formatDateTime(iso: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso))
}

const RELATIVE_STEPS: readonly [Intl.RelativeTimeFormatUnit, number][] = [
  ['minute', 60],
  ['hour', 24],
  ['day', 7],
]

/**
 * "3 時間前" / "3 hours ago" up to a week, then the date. Under a minute is
 * "たった今" / "now" (numeric: 'auto').
 */
export function formatRelativeTime(iso: string, now: number, locale: string): string {
  let value = Math.round((Date.parse(iso) - now) / 60_000)
  if (value > -1) return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(0, 'second')
  for (const [unit, size] of RELATIVE_STEPS) {
    if (Math.abs(value) < size) return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(value, unit)
    value = Math.round(value / size)
  }
  return formatDate(iso, locale)
}
