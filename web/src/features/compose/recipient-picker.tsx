import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ChevronDown, Clock, Loader2, Search } from 'lucide-react'
import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import type { Candidate } from '../../api/peerbonus'
import { candidatesQuery, recipientsQuery } from '../../api/queries'
import { useText } from '../../i18n'
import { ActionError, Avatar, Button, cn, controlClasses, LoadFailed, Notice, Tooltip, useDebounced } from '../../ui'
import { moveSpatial, pickerPeople } from './recipients'

const DEBOUNCE_MS = 250
const MAX_QUERY_LENGTH = 64
/** Person cards: one per row in a phone's sheet (the email fits), two in the form, three when wider. */
const CARD_GRID = 'grid grid-cols-1 gap-2 @sm:grid-cols-2 @2xl:grid-cols-3'

/**
 * 宛先ピッカー. The search field comes first and is always there; below it 最近 (chips), the
 * caller's teams (cards) and 全員 (cards; shown while searching or without a team, else behind
 * 「全員を見る」). Each person appears once, in the first section that has them. Typing narrows
 * every section at once and, when Directory has more people than were loaded, also asks the
 * server. All people form one tab stop: arrow keys move through them as laid out, Enter picks,
 * ↓ from the field enters them, `/` returns to the field. A pick shows as a chip with 「変更」
 * (Escape there clears it). Directory down leaves the recent people and a server search.
 */
export function RecipientPicker({
  id,
  value,
  invalid,
  autoFocus,
  onChange,
}: {
  id: string
  value: Candidate | null
  invalid?: boolean
  /** Focus the search field when the picker first shows (the form just opened). */
  autoFocus?: boolean
  onChange: (candidate: Candidate | null) => void
}) {
  const { t } = useText()
  const recipients = useQuery(recipientsQuery)
  const [filter, setFilter] = useState('')
  const [everyoneOpen, setEveryoneOpen] = useState(false)
  const [active, setActive] = useState<string | null>(null)
  const searchInput = useRef<HTMLInputElement>(null)
  const area = useRef<HTMLDivElement>(null)
  // After 「変更」 the field takes focus again once it is back on screen.
  const refocus = useRef(false)

  useEffect(() => {
    if (value || !refocus.current) return
    refocus.current = false
    searchInput.current?.focus()
  })

  const data = recipients.data
  const searchOnly = recipients.isError || data?.directoryAvailable === false
  const everyoneMeta = data?.directoryAvailable ? data.everyone : { total: 0, items: [] }
  const serverSearch = searchOnly || everyoneMeta.total > everyoneMeta.items.length
  const query = filter.trim()
  const q = useDebounced(query, DEBOUNCE_MS)
  const search = useQuery({ ...candidatesQuery(q), enabled: !value && serverSearch && q.length > 0, placeholderData: keepPreviousData })

  if (value) {
    return (
      <SelectedRecipient
        id={id}
        candidate={value}
        onClear={() => {
          refocus.current = true
          onChange(null)
        }}
      />
    )
  }

  const remote = serverSearch && query && search.data && !search.isError ? search.data : []
  const people = pickerPeople(data, query, remote)
  const hasTeams = Boolean(data?.directoryAvailable && data.teams.some((team) => team.members.length > 0))
  const everyoneShown = Boolean(query) || searchOnly || !hasTeams || everyoneOpen
  const order = [...people.recent, ...people.teams.flatMap((team) => team.members), ...(everyoneShown ? people.everyone : [])]
  const tabStop = order.some((item) => item.personId === active) ? active : (order[0]?.personId ?? null)
  const searching = serverSearch && query.length > 0 && (q !== query || search.isFetching)
  const listId = `${id}-people`
  const keysId = `${id}-keys`

  const pick = (candidate: Candidate) => {
    setActive(candidate.personId)
    onChange(candidate)
  }

  const onAreaKey = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === '/' && !event.metaKey && !event.ctrlKey && !event.altKey) {
      event.preventDefault()
      searchInput.current?.focus()
      return
    }
    const target = event.target instanceof HTMLElement ? event.target.closest<HTMLElement>('[data-candidate]') : null
    if (!target) return
    const buttons = Array.from(area.current?.querySelectorAll<HTMLElement>('[data-candidate]') ?? [])
    const index = buttons.indexOf(target)
    const spots = buttons.map((button) => {
      const rect = button.getBoundingClientRect()
      return { x: rect.left, y: rect.top }
    })
    const next = moveSpatial(event.key, index, spots)
    if (next === undefined) return
    event.preventDefault()
    // Up from the first row goes back to the field.
    if (next === index && event.key === 'ArrowUp') {
      searchInput.current?.focus()
      return
    }
    setActive(buttons[next]?.dataset.candidate ?? null)
    buttons[next]?.focus()
  }

  const card = (candidate: Candidate) => (
    <li key={candidate.personId} className="min-w-0">
      <CandidateButton candidate={candidate} tabStop={candidate.personId === tabStop} onFocus={() => setActive(candidate.personId)} onPick={() => pick(candidate)} />
    </li>
  )

  return (
    <div
      id={id}
      role="group"
      aria-label={t('peerbonus.compose.recipient')}
      aria-describedby={`${id}-${invalid ? 'error' : 'hint'}`}
      className={cn('@container overflow-hidden rounded-md border bg-white', invalid ? 'border-bad' : 'border-line')}
    >
      <p id={keysId} className="sr-only">
        {t('peerbonus.compose.recipientKeys')}
      </p>
      <div className="relative border-b border-line bg-stone-50/70 p-2">
        <Search aria-hidden className="pointer-events-none absolute top-1/2 left-5 size-4 -translate-y-1/2 text-stone-400" />
        <input
          ref={searchInput}
          type="search"
          // The form just opened on purpose; the field is where the next keystroke belongs.
          autoFocus={autoFocus}
          aria-label={t('peerbonus.compose.recipientSearch')}
          aria-controls={listId}
          aria-describedby={keysId}
          placeholder={t('peerbonus.compose.recipientSearch')}
          autoComplete="off"
          maxLength={MAX_QUERY_LENGTH}
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Escape' && filter) {
              event.preventDefault()
              event.stopPropagation()
              setFilter('')
            } else if (event.key === 'ArrowDown') {
              event.preventDefault()
              area.current?.querySelector<HTMLElement>('[data-candidate][tabindex="0"]')?.focus()
            } else if (event.key === 'Enter') {
              // Enter in the field never submits the form; it picks the only match.
              event.preventDefault()
              if (order.length === 1) pick(order[0]!)
            }
          }}
          className={cn(controlClasses, 'h-10 w-full pr-9 pl-9')}
        />
        {searching ? <Loader2 aria-hidden className="absolute top-1/2 right-5 size-4 -translate-y-1/2 animate-spin text-accent" /> : null}
      </div>
      <div ref={area} id={listId} onKeyDown={onAreaKey} className="max-h-80 space-y-4 overflow-y-auto p-3">
        {recipients.isPending ? <PickerSkeleton /> : null}
        {recipients.isError ? <LoadFailed error={recipients.error} onRetry={() => void recipients.refetch()} /> : null}
        {data?.directoryAvailable === false ? <Notice tone="warning">{t('peerbonus.compose.recipientDirectoryDown')}</Notice> : null}
        {people.recent.length > 0 ? (
          <PeopleSection title={t('peerbonus.composer.recent')}>
            <ul aria-label={t('peerbonus.compose.recipientRecent')} className="flex flex-wrap gap-1.5">
              {people.recent.map((candidate) => (
                <li key={candidate.personId} className="min-w-0">
                  <CandidateButton
                    variant="chip"
                    candidate={candidate}
                    tabStop={candidate.personId === tabStop}
                    onFocus={() => setActive(candidate.personId)}
                    onPick={() => pick(candidate)}
                  />
                </li>
              ))}
            </ul>
          </PeopleSection>
        ) : null}
        {people.teams.length > 0 ? (
          <PeopleSection title={t('peerbonus.compose.recipientTeam')}>
            <div className="space-y-3">
              {people.teams.map((team) => (
                <div key={team.group.id}>
                  <h4 title={team.group.name} className="mb-1.5 truncate text-xs font-semibold text-stone-700">
                    {team.group.path}
                  </h4>
                  <ul aria-label={t('peerbonus.compose.recipientTeamMembers', { team: team.group.path })} className={CARD_GRID}>
                    {team.members.map(card)}
                  </ul>
                </div>
              ))}
            </div>
          </PeopleSection>
        ) : null}
        {data?.directoryAvailable && !everyoneShown && everyoneMeta.total > 0 ? (
          <button
            type="button"
            aria-expanded={false}
            onClick={() => setEveryoneOpen(true)}
            className="-ml-1 inline-flex min-h-10 items-center gap-1.5 rounded-md px-1 text-sm font-medium text-accent-strong hover:underline focus-visible:outline-2 focus-visible:outline-accent md:min-h-8"
          >
            {t('peerbonus.composer.showEveryone', { count: everyoneMeta.total })}
            <ChevronDown aria-hidden className="size-4" />
          </button>
        ) : null}
        {everyoneShown && people.everyone.length > 0 ? (
          <PeopleSection
            title={t('peerbonus.composer.everyone')}
            action={
              everyoneOpen && hasTeams && !query ? (
                <button type="button" aria-expanded onClick={() => setEveryoneOpen(false)} className="text-xs font-medium text-stone-500 hover:text-stone-800 hover:underline">
                  {t('peerbonus.composer.hideEveryone')}
                </button>
              ) : null
            }
          >
            <ul aria-label={t('peerbonus.compose.recipientEveryoneList')} className={CARD_GRID}>
              {people.everyone.map(card)}
            </ul>
            {!query && everyoneMeta.total > everyoneMeta.items.length ? (
              <p className="mt-2 text-xs text-stone-500">{t('peerbonus.compose.recipientPartial', { shown: everyoneMeta.items.length, total: everyoneMeta.total })}</p>
            ) : null}
          </PeopleSection>
        ) : null}
        {query && order.length === 0 && !search.isError ? (
          <p role="status" className="text-sm text-stone-500">
            {searching ? t('peerbonus.compose.recipientSearching') : t('peerbonus.compose.recipientNone', { query })}
          </p>
        ) : null}
        {query && search.isError && !searching ? <ActionError error={search.error} /> : null}
      </div>
    </div>
  )
}

function PeopleSection({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section aria-label={title}>
      <div className="mb-2 flex items-baseline justify-between gap-3">
        <h3 className="text-xs font-medium text-stone-500">{title}</h3>
        {action}
      </div>
      {children}
    </section>
  )
}

/** 未ログイン: a small clock with the explanation as a tooltip (and for screen readers). */
function PendingMark() {
  const { t } = useText()
  return (
    <Tooltip content={t('peerbonus.composer.pendingTip')}>
      <span className="inline-flex shrink-0 text-stone-400">
        <Clock aria-hidden className="size-3.5" />
        <span className="sr-only">{t('peerbonus.common.pending')}</span>
      </span>
    </Tooltip>
  )
}

/** The chosen person: name, first known group and email, and 「変更」 (Escape inside also clears). */
function SelectedRecipient({ id, candidate, onClear }: { id: string; candidate: Candidate; onClear: () => void }) {
  const { t } = useText()
  const group = candidate.groups[0]
  const detail = group ? `${group.path} · ${candidate.email}` : candidate.email
  return (
    <div
      id={id}
      role="group"
      aria-label={t('peerbonus.compose.recipient')}
      onKeyDown={(event) => {
        if (event.key !== 'Escape') return
        event.preventDefault()
        event.stopPropagation()
        onClear()
      }}
      className="flex min-h-12 items-center gap-3 rounded-md border border-accent-line bg-accent-soft/60 py-2 pr-1.5 pl-2.5"
    >
      <Avatar name={candidate.displayName} avatarUrl={candidate.avatarUrl} size="md" />
      <div className="min-w-0 flex-1">
        <p className="flex min-w-0 items-center gap-1.5 font-medium text-stone-900">
          <span className="truncate">{candidate.displayName}</span>
          {candidate.pending ? <PendingMark /> : null}
        </p>
        <p className="truncate text-xs text-stone-500" title={detail}>
          {detail}
        </p>
      </div>
      <Button size="sm" tone="ghost" aria-label={t('peerbonus.compose.recipientChangeLabel', { name: candidate.displayName })} onClick={onClear}>
        {t('peerbonus.compose.recipientChange')}
      </Button>
    </div>
  )
}

/** One person: a card (avatar, name, email on its own line) or a recent chip (avatar, name). */
function CandidateButton({
  candidate,
  variant = 'card',
  tabStop,
  onFocus,
  onPick,
}: {
  candidate: Candidate
  variant?: 'card' | 'chip'
  tabStop: boolean
  onFocus: () => void
  onPick: () => void
}) {
  return (
    <button
      type="button"
      data-candidate={candidate.personId}
      tabIndex={tabStop ? 0 : -1}
      onFocus={onFocus}
      onClick={onPick}
      className={cn(
        'border border-line bg-white text-left text-stone-900 transition-colors hover:border-accent-line hover:bg-accent-soft focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent',
        variant === 'chip'
          ? 'inline-flex min-h-10 max-w-full items-center gap-2 rounded-full py-1 pr-3 pl-1 text-sm md:min-h-9'
          : 'flex h-full w-full min-w-0 items-center gap-2.5 rounded-md px-2.5 py-2'
      )}
    >
      <Avatar name={candidate.displayName} avatarUrl={candidate.avatarUrl} size={variant === 'chip' ? 'sm' : 'md'} />
      {variant === 'chip' ? (
        <span className="flex min-w-0 items-center gap-1">
          <span className="truncate font-medium">{candidate.displayName}</span>
          {candidate.pending ? <PendingMark /> : null}
        </span>
      ) : (
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-center gap-1">
            <span className="truncate text-sm font-medium">{candidate.displayName}</span>
            {candidate.pending ? <PendingMark /> : null}
          </span>
          <span className="block truncate text-xs text-stone-500" title={candidate.email}>
            {candidate.email}
          </span>
        </span>
      )}
    </button>
  )
}

/** Placeholder cards while `GET /recipients` loads. */
function PickerSkeleton() {
  const { t } = useText()
  return (
    <div role="status" aria-busy="true">
      <span className="sr-only">{t('peerbonus.compose.recipientLoading')}</span>
      <div aria-hidden className={CARD_GRID}>
        {[0, 1, 2, 3].map((key) => (
          <div key={key} className="flex items-center gap-2.5 rounded-md border border-line bg-white px-2.5 py-2">
            <span className="size-8 shrink-0 animate-pulse rounded-full bg-stone-200" />
            <span className="flex-1 space-y-1.5">
              <span className="block h-3 w-2/3 animate-pulse rounded bg-stone-200" />
              <span className="block h-2.5 w-full animate-pulse rounded bg-stone-100" />
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}
