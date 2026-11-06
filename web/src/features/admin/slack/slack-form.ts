// The Slack settings form: a draft of the server's settings, the reducer the
// controls dispatch to, the checks the server would refuse (design §5 PATCH),
// and the patch of only what changed.
import type { SlackChannelRef, SlackLocale, SlackSettings, SlackSettingsPatch } from '../../../api/peerbonus'

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
