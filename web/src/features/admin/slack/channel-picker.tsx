// 告知チャンネル: a combobox over Slack's public channels (`GET /admin/slack/channels?q=`, prefix,
// debounced). The chosen channel shows with whether the bot is in it — Slack refuses a post to a
// channel the bot was not invited to, so the hint names the `/invite` to run.
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Hash, Loader2 } from 'lucide-react'
import { useId, useState, type KeyboardEvent } from 'react'
import type { SlackChannelRef } from '../../../api/peerbonus'
import { useText } from '../../../i18n'
import { Badge, Button, cn, controlClasses, errorMessage, useDebounced } from '../../../ui'
import { slackChannelsQuery } from '../queries'

const DEBOUNCE_MS = 250

/** `id` is the search field's; the caller renders the label as `${id}-label`. */
export function ChannelPicker({
  id,
  value,
  error,
  disabled,
  onChange,
}: {
  id: string
  value: SlackChannelRef | null
  error?: string
  disabled?: boolean
  onChange: (channel: SlackChannelRef | null) => void
}) {
  const { t } = useText()
  const [editing, setEditing] = useState(value === null)
  const hintId = `${id}-hint`
  const errorId = `${id}-error`

  if (!editing && value) {
    return (
      <div role="group" aria-labelledby={`${id}-label`}>
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex min-h-10 min-w-0 items-center gap-1 rounded-md bg-stone-100 px-3 text-sm font-medium text-stone-900 md:min-h-9">
            <Hash aria-hidden className="size-4 shrink-0 text-stone-500" />
            <span className="truncate">{value.name}</span>
          </span>
          <Button size="sm" disabled={disabled} onClick={() => setEditing(true)}>
            {t('peerbonus.slack.channel.change')}
          </Button>
          <Button size="sm" tone="ghost" disabled={disabled} onClick={() => onChange(null)}>
            {t('peerbonus.slack.channel.remove')}
          </Button>
        </div>
        <Membership channel={value} hintId={hintId} />
      </div>
    )
  }

  return (
    <div>
      <ChannelSearch
        id={id}
        describedBy={error ? errorId : hintId}
        invalid={Boolean(error)}
        disabled={disabled}
        onPick={(channel) => {
          onChange(channel)
          setEditing(false)
        }}
        onCancel={value ? () => setEditing(false) : undefined}
      />
      {error ? (
        <p id={errorId} className="mt-1.5 text-xs text-bad-ink">
          {error}
        </p>
      ) : (
        <p id={hintId} className="mt-1.5 text-xs text-stone-500">
          {t('peerbonus.slack.channel.hint')}
        </p>
      )}
    </div>
  )
}

/** Whether the bot is in the chosen channel, from the channel list (the saved settings only keep id and name). */
function Membership({ channel, hintId }: { channel: SlackChannelRef; hintId: string }) {
  const { t } = useText()
  const channels = useQuery(slackChannelsQuery(channel.name))
  const known = channels.data?.find((candidate) => candidate.id === channel.id)
  if (!known) return null
  return (
    <p id={hintId} className={cn('mt-1.5 text-xs', known.isMember ? 'text-stone-500' : 'text-busy-ink')}>
      {t(known.isMember ? 'peerbonus.slack.channel.joined' : 'peerbonus.slack.channel.invite')}
    </p>
  )
}

/** The search field and its listbox; ↑/↓ move, Enter picks, Escape cancels a change. */
function ChannelSearch({
  id,
  describedBy,
  invalid,
  disabled,
  onPick,
  onCancel,
}: {
  id: string
  describedBy: string
  invalid: boolean
  disabled?: boolean
  onPick: (channel: SlackChannelRef) => void
  onCancel?: () => void
}) {
  const { t } = useText()
  const listId = useId()
  const [text, setText] = useState('')
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const q = useDebounced(text.trim().replace(/^#/, ''), DEBOUNCE_MS)
  const channels = useQuery({ ...slackChannelsQuery(q), enabled: open && !disabled, placeholderData: keepPreviousData })
  const items = channels.data ?? []
  const activeIndex = Math.min(active, items.length - 1)
  const failed = channels.isError ? errorMessage(channels.error) : undefined

  const pick = (index: number) => {
    const channel = items[index]
    if (!channel) return
    onPick({ id: channel.id, name: channel.name })
    setOpen(false)
  }
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      setOpen(true)
      if (items.length > 0) setActive((activeIndex + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length)
    } else if (event.key === 'Enter') {
      if (!open) return
      event.preventDefault()
      pick(activeIndex)
    } else if (event.key === 'Escape') {
      if (open) {
        event.preventDefault()
        setOpen(false)
      } else {
        onCancel?.()
      }
    }
  }

  return (
    <div className="relative max-w-sm">
      <Hash aria-hidden className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-stone-400" />
      <input
        id={id}
        role="combobox"
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={open && activeIndex >= 0 ? `${listId}-${activeIndex}` : undefined}
        aria-labelledby={`${id}-label`}
        aria-describedby={describedBy}
        // Opened by 「変更」: the field takes focus at once.
        autoFocus={onCancel !== undefined}
        aria-invalid={invalid || undefined}
        autoComplete="off"
        spellCheck={false}
        disabled={disabled}
        placeholder={t('peerbonus.slack.channel.search')}
        value={text}
        onChange={(event) => {
          setText(event.target.value)
          setActive(0)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => {
          setOpen(false)
          if (text === '') onCancel?.()
        }}
        onKeyDown={onKeyDown}
        className={cn(controlClasses, 'h-10 w-full pr-9 pl-9 md:h-9')}
      />
      {channels.isFetching ? <Loader2 aria-hidden className="absolute top-1/2 right-3 size-4 -translate-y-1/2 animate-spin text-stone-400" /> : null}
      {open ? (
        <div className="absolute inset-x-0 top-full z-20 mt-1 max-h-64 overflow-y-auto rounded-md border border-stone-200 bg-white py-1 shadow-lg">
          <ul id={listId} role="listbox" aria-label={t('peerbonus.slack.channel.options')}>
            {items.map((channel, index) => (
              <li
                key={channel.id}
                id={`${listId}-${index}`}
                role="option"
                aria-selected={index === activeIndex}
                // Keep focus in the field so the blur does not close the list before the click lands.
                onMouseDown={(event) => event.preventDefault()}
                onMouseEnter={() => setActive(index)}
                onClick={() => pick(index)}
                className={cn('flex min-h-10 cursor-pointer items-center justify-between gap-3 px-3 text-sm md:min-h-8', index === activeIndex ? 'bg-accent-soft' : undefined)}
              >
                <span className="flex min-w-0 items-center gap-1 text-stone-900">
                  <Hash aria-hidden className="size-3.5 shrink-0 text-stone-400" />
                  <span className="truncate">{channel.name}</span>
                </span>
                {channel.isMember ? null : <Badge>{t('peerbonus.slack.channel.notJoined')}</Badge>}
              </li>
            ))}
          </ul>
          {channels.isSuccess && items.length === 0 ? <p className="px-3 py-2 text-sm text-stone-500">{t('peerbonus.slack.channel.none', { q })}</p> : null}
          {failed ? <p className="px-3 py-2 text-sm text-bad-ink">{t(failed.id, failed.values)}</p> : null}
        </div>
      ) : null}
    </div>
  )
}
