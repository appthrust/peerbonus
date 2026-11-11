// The Slack settings forms: drafts of the server's settings, the reducers the
// controls dispatch to, the checks the server would refuse (design §5 PATCH,
// reaction-give §5), and the patch of only what changed. Two cards edit the
// settings: 配信設定 (delivery) and 会話の中で贈る (giving in a conversation).
import type { SlackChannelRef, SlackLocale, SlackReactionRule, SlackSettings, SlackSettingsPatch } from '../../../api/peerbonus'

export type SlackDraft = SlackSettings

export type SlackToggle = 'announcePosts' | 'dmRecipientOnPost' | 'giveFromSlack'

export type SlackDraftAction =
  | { type: 'channel'; channel: SlackChannelRef | null }
  | { type: 'toggle'; field: SlackToggle; value: boolean }
  | { type: 'locale'; locale: SlackLocale }
  | { type: 'reset'; settings: SlackSettings }

/**
 * Picking the first channel also turns announcements on (that is why one picks it); clearing the
 * channel turns them off, since the server refuses announcements without one.
 */
export function slackDraftReducer(draft: SlackDraft, action: SlackDraftAction): SlackDraft {
  switch (action.type) {
    case 'channel':
      if (action.channel === null) return { ...draft, announceChannel: null, announcePosts: false }
      return { ...draft, announceChannel: action.channel, announcePosts: draft.announceChannel === null ? true : draft.announcePosts }
    case 'toggle':
      return { ...draft, [action.field]: action.value }
    case 'locale':
      return { ...draft, locale: action.locale }
    case 'reset':
      return action.settings
  }
}

/** What the server would refuse: announcements without a channel (400), giving from Slack without the signing secret (409). */
export type SlackDraftProblems = { announceChannel?: 'required'; giveFromSlack?: 'signingSecretMissing' }

export function validateSlackDraft(draft: SlackDraft, { signingSecret }: { signingSecret: boolean }): SlackDraftProblems {
  const problems: SlackDraftProblems = {}
  if (draft.announcePosts && draft.announceChannel === null) problems.announceChannel = 'required'
  if (draft.giveFromSlack && !signingSecret) problems.giveFromSlack = 'signingSecretMissing'
  return problems
}

/** The PATCH body: only the fields that differ from the stored settings (`{}` when nothing changed). */
export function slackPatchOf(draft: SlackDraft, settings: SlackSettings): SlackSettingsPatch {
  const patch: SlackSettingsPatch = {}
  if ((draft.announceChannel?.id ?? null) !== (settings.announceChannel?.id ?? null)) patch.announceChannelId = draft.announceChannel?.id ?? null
  for (const field of ['announcePosts', 'dmRecipientOnPost', 'giveFromSlack'] as const) if (draft[field] !== settings[field]) patch[field] = draft[field]
  if (draft.locale !== settings.locale) patch.locale = draft.locale
  return patch
}

/** What the delivery card edits, as a stable string: its form restarts when a save changed it. */
export const deliveryKey = (settings: SlackSettings) =>
  JSON.stringify([settings.announceChannel?.id ?? null, settings.announcePosts, settings.dmRecipientOnPost, settings.giveFromSlack, settings.locale])

// ---- 会話の中で贈る (mentions and reaction emoji) ----

/** domain.SlackMaxPointChoices / SlackMaxReactionRules. */
export const MAX_POINT_CHOICES = 5
export const MAX_REACTION_RULES = 8
const MAX_EMOJI_LENGTH = 100
const WHOLE = /^\d+$/

/** Numbers stay text while typing; `rowId` keeps React keys and problems stable. `valueKey: ''` is no value. */
export type PointChoiceDraft = { rowId: string; points: string }
export type ReactionRuleDraft = { rowId: string; emoji: string; points: string; valueKey: string }
export type ConversationDraft = { giveInConversation: boolean; choices: PointChoiceDraft[]; rules: ReactionRuleDraft[] }

/** Add actions carry the new row's id (`newRowId`), so the caller can focus it and the reducer stays pure. */
export type ConversationAction =
  | { type: 'toggle'; value: boolean }
  | { type: 'addChoice'; rowId: string }
  | { type: 'setChoice'; rowId: string; points: string }
  | { type: 'removeChoice'; rowId: string }
  | { type: 'addRule'; rowId: string }
  | { type: 'setRule'; rowId: string; change: Partial<Omit<ReactionRuleDraft, 'rowId'>> }
  | { type: 'removeRule'; rowId: string }
  | { type: 'reset'; settings: SlackSettings }

let rowSeq = 0
/** A fresh local row id (never sent to the server). */
export const newRowId = (prefix: 'choice' | 'rule') => `${prefix}-${++rowSeq}`

export function conversationDraftOf(settings: SlackSettings): ConversationDraft {
  return {
    giveInConversation: settings.giveInConversation,
    choices: settings.mentionPointChoices.map((points) => ({ rowId: newRowId('choice'), points: String(points) })),
    rules: settings.reactionRules.map((rule) => ({ rowId: newRowId('rule'), emoji: rule.emoji, points: String(rule.points), valueKey: rule.valueKey ?? '' })),
  }
}

/** What the conversation card edits, as a stable string (see deliveryKey). */
export const conversationKey = (settings: SlackSettings) => JSON.stringify([settings.giveInConversation, settings.mentionPointChoices, settings.reactionRules])

/** Adding stops at the limits (1–5 choices, at most 8 rules); the last choice cannot go. */
export function conversationReducer(draft: ConversationDraft, action: ConversationAction): ConversationDraft {
  switch (action.type) {
    case 'toggle':
      return { ...draft, giveInConversation: action.value }
    case 'addChoice':
      return draft.choices.length >= MAX_POINT_CHOICES ? draft : { ...draft, choices: [...draft.choices, { rowId: action.rowId, points: '' }] }
    case 'setChoice':
      return { ...draft, choices: draft.choices.map((row) => (row.rowId === action.rowId ? { ...row, points: action.points } : row)) }
    case 'removeChoice':
      return draft.choices.length <= 1 ? draft : { ...draft, choices: draft.choices.filter((row) => row.rowId !== action.rowId) }
    case 'addRule':
      return draft.rules.length >= MAX_REACTION_RULES ? draft : { ...draft, rules: [...draft.rules, { rowId: action.rowId, emoji: '', points: '', valueKey: '' }] }
    case 'setRule':
      return { ...draft, rules: draft.rules.map((row) => (row.rowId === action.rowId ? { ...row, ...action.change } : row)) }
    case 'removeRule':
      return { ...draft, rules: draft.rules.filter((row) => row.rowId !== action.rowId) }
    case 'reset':
      return conversationDraftOf(action.settings)
  }
}

/** As domain.NormalizeSlackEmoji: no colons, no skin tone (":+1::skin-tone-3:" → "+1"), lower case. */
export function normalizeSlackEmoji(name: string): string {
  let bare = name.trim().toLowerCase()
  const tone = bare.indexOf('::')
  if (tone >= 0) bare = bare.slice(0, tone)
  return bare.replace(/^:+|:+$/g, '')
}

/** Emoji people put on messages every day: a rule for one would give points by accident. */
const EVERYDAY_EMOJI: Record<string, true> = {
  '+1': true,
  thumbsup: true,
  '-1': true,
  thumbsdown: true,
  heart: true,
  pray: true,
  bow: true,
  ok_hand: true,
  ok: true,
  eyes: true,
  white_check_mark: true,
  heavy_check_mark: true,
  clap: true,
  tada: true,
  raised_hands: true,
  joy: true,
  smile: true,
  laughing: true,
  '100': true,
  fire: true,
  muscle: true,
  sparkles: true,
  sob: true,
}

export const isEverydayEmoji = (emoji: string) => EVERYDAY_EMOJI[normalizeSlackEmoji(emoji)] === true

export type RuleProblems = { emoji?: 'required' | 'invalid' | 'duplicate'; points?: 'range'; valueKey?: 'unknown' }
/**
 * Per `rowId`. `giveInConversation`: on without the signing secret, which the server refuses (409). The
 * missing inbound URL only keeps the switch from being turned on: the server does not know about it.
 */
export type ConversationProblems = {
  giveInConversation?: 'signingSecretMissing'
  choices?: Record<string, 'range' | 'duplicate'>
  rules?: Record<string, RuleProblems>
}

const pointsIn = (text: string, max: number) => WHOLE.test(text.trim()) && Number(text) >= 1 && Number(text) <= max

/** domain.ValidateMentionPointChoices / ValidateSlackReactionRules against the Program, and the signing secret the switch needs. */
export function validateConversation(
  draft: ConversationDraft,
  { signingSecret, maxPointsPerPost, valueKeys }: { signingSecret: boolean; maxPointsPerPost: number; valueKeys: readonly string[] }
): ConversationProblems {
  const problems: ConversationProblems = {}
  if (draft.giveInConversation && !signingSecret) problems.giveInConversation = 'signingSecretMissing'
  const choices: Record<string, 'range' | 'duplicate'> = {}
  draft.choices.forEach((row, index) => {
    if (!pointsIn(row.points, maxPointsPerPost)) choices[row.rowId] = 'range'
    else if (draft.choices.slice(0, index).some((other) => Number(other.points) === Number(row.points))) choices[row.rowId] = 'duplicate'
  })
  if (Object.keys(choices).length > 0) problems.choices = choices
  const rules: Record<string, RuleProblems> = {}
  draft.rules.forEach((row, index) => {
    const rule: RuleProblems = {}
    const emoji = normalizeSlackEmoji(row.emoji)
    if (emoji === '') rule.emoji = 'required'
    else if (emoji.length > MAX_EMOJI_LENGTH || /[:\s]/.test(emoji)) rule.emoji = 'invalid'
    else if (draft.rules.slice(0, index).some((other) => normalizeSlackEmoji(other.emoji) === emoji)) rule.emoji = 'duplicate'
    if (!pointsIn(row.points, maxPointsPerPost)) rule.points = 'range'
    if (row.valueKey !== '' && !valueKeys.includes(row.valueKey)) rule.valueKey = 'unknown'
    if (Object.keys(rule).length > 0) rules[row.rowId] = rule
  })
  if (Object.keys(rules).length > 0) problems.rules = rules
  return problems
}

const sameRules = (a: SlackReactionRule[], b: SlackReactionRule[]) =>
  a.length === b.length && a.every((rule, index) => rule.emoji === b[index]!.emoji && rule.points === b[index]!.points && rule.valueKey === b[index]!.valueKey)

/** The PATCH body of the conversation card; the lists are sent whole (the server replaces them). `{}` when nothing changed. */
export function conversationPatchOf(draft: ConversationDraft, settings: SlackSettings): SlackSettingsPatch {
  const patch: SlackSettingsPatch = {}
  if (draft.giveInConversation !== settings.giveInConversation) patch.giveInConversation = draft.giveInConversation
  const choices = draft.choices.map((row) => Number(row.points.trim()))
  if (choices.length !== settings.mentionPointChoices.length || choices.some((points, index) => points !== settings.mentionPointChoices[index])) patch.mentionPointChoices = choices
  const rules = draft.rules.map((row) => ({ emoji: normalizeSlackEmoji(row.emoji), points: Number(row.points.trim()), valueKey: row.valueKey === '' ? null : row.valueKey }))
  if (!sameRules(rules, settings.reactionRules)) patch.reactionRules = rules
  return patch
}
