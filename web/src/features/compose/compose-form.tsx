import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Send } from 'lucide-react'
import { useEffect, useId, useLayoutEffect, useRef, useState, type FormEvent } from 'react'
import { isPeerbonusError, newIdempotencyKey, peerbonusApi, type Candidate, type Me, type Post } from '../../api/peerbonus'
import { cachedMember, invalidatePeerbonus, meQuery, recipientsQuery } from '../../api/queries'
import type { MessageId } from '../../copy'
import { useText } from '../../i18n'
import { clampPoints, composeProblems, MAX_MESSAGE_LENGTH, MAX_VALUES, textLength, type ComposeProblems } from '../../model/post'
import { useSession } from '../../shell/use-session'
import { ActionError, Button, cn, controlClasses, Field, Notice } from '../../ui'
import { PointInput } from './point-input'
import { RecipientPicker } from './recipient-picker'
import { findRecipient } from './recipients'
import { ValueChips } from './value-chips'

const DEFAULT_POINTS = 30

const PROBLEM_TEXT: Record<NonNullable<ComposeProblems[keyof ComposeProblems]>, MessageId> = {
  recipientRequired: 'peerbonus.compose.recipientRequired',
  pointsRange: 'peerbonus.compose.pointsRange',
  pointsInsufficient: 'peerbonus.compose.pointsInsufficient',
  messageRequired: 'peerbonus.compose.messageRequired',
  messageTooLong: 'peerbonus.compose.messageTooLong',
  messageControl: 'peerbonus.compose.messageControl',
  valuesTooMany: 'peerbonus.compose.valuesTooMany',
}

/** What the form holds; the launcher keeps it while the form is closed. */
export type ComposeDraft = { recipient: Candidate | null; points: number; message: string; values: string[] }

/**
 * 送付フォーム: the recipient picker, the point stepper (1..this week's per-post cap) with what
 * sending leaves of this week's points, the auto-growing message with its counter, and the
 * Program's values. The balance itself lives in the header. Problems show after the first send
 * attempt. One draft keeps its `Idempotency-Key` until it changes or is sent, so resending after
 * an unanswered attempt cannot send twice. A send moves the header balance at once, resets the
 * draft and calls `onSent`.
 *
 * `to` (`/?to=<personId>`, 「贈る」 on other screens) fills the recipient once the picker's people
 * are known and focuses the points; `onPreselected` then lets the page drop `to` from the URL.
 * `draft` / `onDraftChange` carry the draft across the form closing and opening again.
 */
export function ComposeForm({
  to,
  onPreselected,
  onSent,
  onCancel,
  autoFocus = false,
  draft: savedDraft,
  onDraftChange,
  labelledBy,
}: {
  to?: string
  onPreselected?: () => void
  onSent?: (post: Post) => void
  /** Shows キャンセル beside the send button. */
  onCancel?: () => void
  /** Focus where the next keystroke belongs (search, else the message) when the form mounts. */
  autoFocus?: boolean
  draft?: ComposeDraft | null
  onDraftChange?: (draft: ComposeDraft | null) => void
  labelledBy?: string
} = {}) {
  const { t } = useText()
  const { me } = useSession()
  const client = useQueryClient()
  const id = useId()
  const pointsId = `${id}-points`
  const messageId = `${id}-message`
  const max = me.period.maxPointsPerPost
  const { remaining } = me.stats.period
  const initial = (): ComposeDraft => ({ recipient: null, points: clampPoints(Math.min(DEFAULT_POINTS, Math.max(1, remaining)), max), message: '', values: [] })
  const [draft, setDraft] = useState<ComposeDraft>(() => savedDraft ?? initial())
  const [attempted, setAttempted] = useState(false)
  const attemptKey = useRef<string | null>(null)
  const form = useRef<HTMLFormElement>(null)
  const message = useRef<HTMLTextAreaElement>(null)
  const recipients = useQuery(recipientsQuery)
  const [appliedTo, setAppliedTo] = useState<string | undefined>(undefined)
  const [arrival, setArrival] = useState<'found' | 'missing' | null>(null)

  // Applied while rendering, so the first paint (and static markup) already shows the chosen person.
  if (to !== appliedTo && (!to || !recipients.isPending)) {
    setAppliedTo(to)
    if (to) {
      const recipient = findRecipient(to, recipients.data, cachedMember(client, to), me.member.personId)
      if (recipient) setDraft((current) => ({ ...current, recipient }))
      setArrival(recipient ? 'found' : 'missing')
    }
  }

  const {
    mutate,
    reset,
    isPending,
    isError,
    error: sendError,
  } = useMutation({
    mutationFn: (input: ComposeDraft) => {
      attemptKey.current ??= newIdempotencyKey()
      return peerbonusApi.posts.create(
        { recipientPersonId: input.recipient!.personId, points: input.points, message: input.message.trim(), values: input.values },
        attemptKey.current
      )
    },
    onSuccess: async (post) => {
      attemptKey.current = null
      setDraft(initial())
      setAttempted(false)
      onDraftChange?.(null)
      // The header ring moves now; the refetch below confirms the server's numbers.
      client.setQueryData<Me>(meQuery.queryKey, (current) => {
        if (!current) return current
        const period = current.stats.period
        return {
          ...current,
          stats: {
            ...current.stats,
            period: { ...period, remaining: Math.max(0, period.remaining - post.points), sentPosts: period.sentPosts + 1, sentPoints: period.sentPoints + post.points },
          },
        }
      })
      onSent?.(post)
      await invalidatePeerbonus(client)
    },
    onError: async (error) => {
      // The server's balance differs from ours; show its number.
      if (isPeerbonusError(error, 'insufficient_points')) await client.invalidateQueries({ queryKey: meQuery.queryKey })
    },
  })

  useEffect(() => {
    if (!arrival) return
    setArrival(null)
    if (arrival === 'found') {
      // A new recipient is a new draft.
      attemptKey.current = null
      if (!isPending) reset()
      document.getElementById(pointsId)?.focus({ preventScroll: true })
    }
    form.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    onPreselected?.()
  }, [arrival, isPending, reset, pointsId, onPreselected])

  // The message grows with its text (up to the CSS max height, then it scrolls).
  useLayoutEffect(() => {
    const area = message.current
    if (!area) return
    area.style.height = 'auto'
    area.style.height = `${area.scrollHeight + 2}px`
  }, [draft.message])

  const update = (patch: Partial<ComposeDraft>) => {
    // A changed draft is a new send.
    attemptKey.current = null
    if (!isPending) reset()
    const next = { ...draft, ...patch }
    setDraft(next)
    onDraftChange?.(next)
  }

  const problems = composeProblems({ recipientPersonId: draft.recipient?.personId ?? null, points: draft.points, message: draft.message, values: draft.values }, { max, remaining })
  const shown = attempted ? problems : {}
  const problemText = (problem: keyof ComposeProblems) => {
    const code = shown[problem]
    return code ? t(PROBLEM_TEXT[code], { max: problem === 'message' ? MAX_MESSAGE_LENGTH : problem === 'values' ? MAX_VALUES : max, remaining }) : undefined
  }

  const submit = (event: FormEvent) => {
    event.preventDefault()
    setAttempted(true)
    if (Object.keys(problems).length > 0 || isPending) return
    mutate(draft)
  }

  const points = Number.isFinite(draft.points) ? draft.points : 0
  const left = remaining - points
  const messageLength = textLength(draft.message.trim())
  const valuesLabelId = `${id}-values-label`

  return (
    <form ref={form} noValidate onSubmit={submit} aria-labelledby={labelledBy} className="scroll-mt-20 space-y-4">
      {remaining === 0 ? <Notice>{t('peerbonus.compose.empty')}</Notice> : null}
      <Field
        id={`${id}-recipient`}
        label={t('peerbonus.compose.recipient')}
        error={problemText('recipient')}
        hint={draft.recipient?.pending ? t('peerbonus.compose.recipientPendingHint') : undefined}
      >
        <RecipientPicker
          id={`${id}-recipient`}
          value={draft.recipient}
          invalid={Boolean(shown.recipient)}
          autoFocus={autoFocus}
          onChange={(recipient) => {
            update({ recipient })
            // Picked: on to the points.
            if (recipient) requestAnimationFrame(() => document.getElementById(pointsId)?.focus())
          }}
        />
      </Field>
      <Field
        id={pointsId}
        label={t('peerbonus.compose.points')}
        error={problemText('points')}
        hint={
          left >= 0 ? (
            t('peerbonus.composer.after', { remaining: left })
          ) : (
            <span className="font-medium text-bad-ink">{t('peerbonus.compose.pointsInsufficient', { remaining })}</span>
          )
        }
      >
        <PointInput id={pointsId} value={draft.points} max={max} invalid={Boolean(shown.points)} onChange={(next) => update({ points: next })} />
      </Field>
      <Field
        id={messageId}
        label={t('peerbonus.compose.message')}
        error={problemText('message')}
        aside={
          <span className={cn(messageLength > MAX_MESSAGE_LENGTH ? 'font-semibold text-bad-ink' : undefined)}>
            {t('peerbonus.compose.messageCount', { count: messageLength, max: MAX_MESSAGE_LENGTH })}
          </span>
        }
      >
        <textarea
          ref={message}
          id={messageId}
          // A draft brought back with its recipient continues at the message (without one, the picker's search takes focus).
          autoFocus={autoFocus && Boolean(savedDraft?.recipient) && !to}
          rows={3}
          value={draft.message}
          onChange={(event) => update({ message: event.target.value })}
          placeholder={t('peerbonus.compose.messagePlaceholder')}
          aria-invalid={shown.message ? true : undefined}
          aria-describedby={shown.message ? `${messageId}-error` : undefined}
          className={cn(controlClasses, 'max-h-72 min-h-20 w-full resize-none overflow-y-auto py-2 leading-relaxed')}
        />
      </Field>
      {me.program.values.length > 0 ? (
        <div>
          <div className="flex items-baseline justify-between gap-3">
            <span id={valuesLabelId} className="flex items-center gap-2 text-sm font-medium text-stone-800">
              {t('peerbonus.compose.values')}
              <span className="text-xs font-normal text-stone-500">{t('peerbonus.composer.optional')}</span>
            </span>
            <span className="text-xs text-stone-500 tabular-nums">{t('peerbonus.compose.valuesHint', { max: MAX_VALUES })}</span>
          </div>
          <div className="mt-1.5">
            <ValueChips
              labelledBy={valuesLabelId}
              describedBy={shown.values ? `${id}-values-error` : undefined}
              values={me.program.values}
              selected={draft.values}
              onChange={(values) => update({ values })}
            />
          </div>
          {shown.values ? (
            <p id={`${id}-values-error`} className="mt-1.5 text-xs text-bad-ink">
              {problemText('values')}
            </p>
          ) : null}
        </div>
      ) : null}
      {isError ? <ActionError error={sendError} /> : null}
      <div className="flex flex-col-reverse gap-2 border-t border-line pt-4 sm:flex-row sm:items-center sm:justify-end">
        {onCancel ? (
          <Button tone="ghost" onClick={onCancel} disabled={isPending}>
            {t('peerbonus.common.cancel')}
          </Button>
        ) : null}
        <Button type="submit" tone="primary" icon={<Send />} loading={isPending} disabled={remaining === 0} className="max-w-full min-w-0">
          <span className="truncate">
            {draft.recipient ? t('peerbonus.compose.submitTo', { name: draft.recipient.displayName, points }) : t('peerbonus.compose.submit', { points })}
          </span>
        </Button>
      </div>
    </form>
  )
}
