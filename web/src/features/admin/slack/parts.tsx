// Small building blocks of `/admin/slack`: the card each concern sits in, a
// tone chip for states, an on/off switch row and a read-only value with a copy
// button.
import { Check, Copy } from 'lucide-react'
import { useEffect, useId, useState, type ReactNode } from 'react'
import { useText } from '../../../i18n'
import { Button, cn, controlClasses } from '../../../ui'
import type { Tone } from './slack-state'

/** One concern of the page: a white card with a heading, an optional lead and actions beside the heading; no body for null children. */
export function Card({ title, lead, actions, className, children }: { title: ReactNode; lead?: ReactNode; actions?: ReactNode; className?: string; children: ReactNode }) {
  const headingId = useId()
  return (
    <section aria-labelledby={headingId} className={cn('rounded-lg border border-line bg-white p-4 sm:p-5', className)}>
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <h2 id={headingId} className="text-base font-semibold text-stone-900">
            {title}
          </h2>
          {lead ? <p className="mt-1 max-w-prose text-sm text-stone-500">{lead}</p> : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
      {children ? <div className="mt-4">{children}</div> : null}
    </section>
  )
}

const TONE_CLASSES: Record<Tone, { chip: string; dot: string }> = {
  ok: { chip: 'bg-ok-soft text-ok ring-emerald-200', dot: 'bg-ok' },
  busy: { chip: 'bg-busy-soft text-busy-ink ring-amber-200', dot: 'bg-busy' },
  bad: { chip: 'bg-bad-soft text-bad-ink ring-red-200', dot: 'bg-bad' },
  neutral: { chip: 'bg-stone-100 text-stone-600 ring-stone-200', dot: 'bg-stone-400' },
}

/** A state label with a coloured dot; the text carries the meaning, the colour only repeats it. */
export function ToneChip({ tone, children }: { tone: Tone; children: string }) {
  const classes = TONE_CLASSES[tone]
  return (
    <span className={cn('inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium whitespace-nowrap ring-1 ring-inset', classes.chip)}>
      <span aria-hidden className={cn('size-1.5 rounded-full', classes.dot)} />
      {children}
    </span>
  )
}

/** A labelled on/off switch (`role="switch"`) with its hint, the switch on the right. */
export function SwitchRow({
  label,
  hint,
  checked,
  disabled,
  onChange,
}: {
  label: string
  hint: ReactNode
  checked: boolean
  disabled?: boolean
  onChange: (checked: boolean) => void
}) {
  const id = useId()
  return (
    <div className="flex items-start justify-between gap-4 py-3">
      <div className="min-w-0">
        <p id={`${id}-label`} className={cn('text-sm font-medium', disabled ? 'text-stone-500' : 'text-stone-800')}>
          {label}
        </p>
        <p id={`${id}-hint`} className="mt-0.5 text-xs text-stone-500">
          {hint}
        </p>
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-labelledby={`${id}-label`}
        aria-describedby={`${id}-hint`}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cn(
          'relative mt-0.5 inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:opacity-50',
          checked ? 'bg-accent' : 'bg-stone-300'
        )}
      >
        <span aria-hidden className={cn('inline-block size-5 rounded-full bg-white shadow-sm transition-transform', checked ? 'translate-x-5.5' : 'translate-x-0.5')} />
      </button>
    </div>
  )
}

const COPIED_MS = 2_000

/** A value to paste elsewhere (a URL, a Dashboard path): read-only, monospace, with a copy button that confirms. */
export function CopyField({ id, value, label }: { id: string; value: string; label: string }) {
  const { t } = useText()
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), COPIED_MS)
    return () => clearTimeout(timer)
  }, [copied])
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
    } catch {
      // No clipboard permission: select the text so a manual copy is one keystroke away.
      const input = document.getElementById(id)
      if (input instanceof HTMLInputElement) input.select()
    }
  }
  return (
    <div className="flex min-w-0 gap-2">
      <input
        id={id}
        readOnly
        value={value}
        aria-label={label}
        onFocus={(event) => event.target.select()}
        className={cn(controlClasses, 'h-10 min-w-0 flex-1 bg-stone-50 font-mono text-xs md:h-9')}
      />
      <Button onClick={() => void copy()} icon={copied ? <Check /> : <Copy />} aria-live="polite">
        {t(copied ? 'peerbonus.slack.copied' : 'peerbonus.slack.copy')}
      </Button>
    </div>
  )
}
