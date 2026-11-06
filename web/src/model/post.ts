// Post and clap write rules (spec.md §3.2–3.3, api.md §4), shared by the
// compose form and the dev mock so both refuse what the server refuses.

export const MAX_MESSAGE_LENGTH = 500
export const MAX_VALUES = 3
export const CLAP_COST = 2
export const VALUE_KEY = /^[a-z0-9-]{1,32}$/

/** Characters as the server counts them (Unicode code points, not UTF-16 units). */
export function textLength(text: string): number {
  return Array.from(text).length
}

// Control characters other than newline and tab (C0, DEL, C1), as Go's unicode.IsControl sees them.
const CONTROL = /[\u0000-\u0008\u000B-\u001F\u007F-\u009F]/

export type MessageProblem = 'messageRequired' | 'messageTooLong' | 'messageControl'

/** What the message breaks after trimming, else undefined. */
export function messageProblem(message: string): MessageProblem | undefined {
  const trimmed = message.trim()
  if (!trimmed) return 'messageRequired'
  if (textLength(trimmed) > MAX_MESSAGE_LENGTH) return 'messageTooLong'
  if (CONTROL.test(trimmed)) return 'messageControl'
  return undefined
}

export type ComposeDraft = {
  recipientPersonId: string | null
  points: number
  message: string
  values: readonly string[]
}

export type ComposeProblems = {
  recipient?: 'recipientRequired'
  points?: 'pointsRange' | 'pointsInsufficient'
  message?: MessageProblem
  values?: 'valuesTooMany'
}

/**
 * What a draft breaks against this week's limits (an empty object when it can be sent).
 * `max` is the frozen per-post maximum, `remaining` the caller's give balance.
 */
export function composeProblems(draft: ComposeDraft, limits: { max: number; remaining: number }): ComposeProblems {
  const problems: ComposeProblems = {}
  if (!draft.recipientPersonId) problems.recipient = 'recipientRequired'
  if (!Number.isInteger(draft.points) || draft.points < 1 || draft.points > limits.max) problems.points = 'pointsRange'
  else if (draft.points > limits.remaining) problems.points = 'pointsInsufficient'
  const message = messageProblem(draft.message)
  if (message) problems.message = message
  if (draft.values.length > MAX_VALUES) problems.values = 'valuesTooMany'
  return problems
}

/** The point stepper's clamp: an integer in 1..max (max itself at least 1). */
export function clampPoints(value: number, max: number): number {
  const top = Math.max(1, max)
  if (!Number.isFinite(value)) return 1
  return Math.min(top, Math.max(1, Math.round(value)))
}
