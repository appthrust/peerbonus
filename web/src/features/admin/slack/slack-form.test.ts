import { describe, expect, test } from 'bun:test'
import type { SlackSettings } from '../../../api/peerbonus'
import {
  conversationDraftOf,
  conversationKey,
  conversationPatchOf,
  conversationReducer,
  deliveryKey,
  isEverydayEmoji,
  newRowId,
  normalizeSlackEmoji,
  slackDraftReducer,
  slackPatchOf,
  validateConversation,
  validateSlackDraft,
  type ConversationAction,
  type SlackDraftAction,
} from './slack-form'

const stored: SlackSettings = {
  announceChannel: null,
  announcePosts: false,
  dmRecipientOnPost: true,
  giveFromSlack: false,
  locale: 'ja',
  giveInConversation: false,
  mentionPointChoices: [10, 30, 50],
  reactionRules: [],
}
const general = { id: 'C0GENERAL', name: 'general' }
const kudos = { id: 'C0KUDOS', name: 'kudos' }

const apply = (settings: SlackSettings, ...actions: SlackDraftAction[]) => actions.reduce(slackDraftReducer, settings)

describe('Slack settings form', () => {
  test('the stored settings are clean: no problems, empty patch', () => {
    expect(validateSlackDraft(stored, { signingSecret: false })).toEqual({})
    expect(slackPatchOf(stored, stored)).toEqual({})
  })

  test('the first channel turns announcements on; a change of channel keeps the switch; clearing turns it off', () => {
    const picked = apply(stored, { type: 'channel', channel: general })
    expect(picked).toMatchObject({ announceChannel: general, announcePosts: true })
    expect(slackPatchOf(picked, stored)).toEqual({ announceChannelId: 'C0GENERAL', announcePosts: true })

    const quiet = { ...stored, announceChannel: general, announcePosts: false }
    expect(apply(quiet, { type: 'channel', channel: kudos })).toMatchObject({ announceChannel: kudos, announcePosts: false })

    const cleared = apply({ ...stored, announceChannel: general, announcePosts: true }, { type: 'channel', channel: null })
    expect(cleared).toMatchObject({ announceChannel: null, announcePosts: false })
    expect(slackPatchOf(cleared, { ...stored, announceChannel: general, announcePosts: true })).toEqual({ announceChannelId: null, announcePosts: false })
  })

  test('announcing without a channel and giving from Slack without the signing secret are problems', () => {
    const noChannel = apply(stored, { type: 'toggle', field: 'announcePosts', value: true })
    expect(validateSlackDraft(noChannel, { signingSecret: false })).toEqual({ announceChannel: 'required' })
    const give = apply(stored, { type: 'toggle', field: 'giveFromSlack', value: true })
    expect(validateSlackDraft(give, { signingSecret: false })).toEqual({ giveFromSlack: 'signingSecretMissing' })
    expect(validateSlackDraft(give, { signingSecret: true })).toEqual({})
  })

  test('the patch carries only what changed; toggling back or reset leaves nothing to save', () => {
    const edited = apply(stored, { type: 'toggle', field: 'dmRecipientOnPost', value: false }, { type: 'locale', locale: 'en' })
    expect(slackPatchOf(edited, stored)).toEqual({ dmRecipientOnPost: false, locale: 'en' })
    expect(slackPatchOf(apply(edited, { type: 'toggle', field: 'dmRecipientOnPost', value: true }, { type: 'locale', locale: 'ja' }), stored)).toEqual({})
    expect(apply(edited, { type: 'reset', settings: stored })).toBe(stored)
    // Re-picking the stored channel (same id) is no change.
    const saved = { ...stored, announceChannel: general, announcePosts: true }
    expect(slackPatchOf(apply(saved, { type: 'channel', channel: { ...general } }), saved)).toEqual({})
  })
})

describe('give in conversation form', () => {
  const program = { signingSecret: true, maxPointsPerPost: 120, valueKeys: ['thanks', 'teamwork'] }
  const withRules: SlackSettings = {
    ...stored,
    reactionRules: [
      { emoji: 'pb-10', points: 10, valueKey: null },
      { emoji: 'pb-30', points: 30, valueKey: 'thanks' },
    ],
  }
  const apply = (settings: SlackSettings, ...actions: ConversationAction[]) => actions.reduce(conversationReducer, conversationDraftOf(settings))

  test('the stored settings are clean; the cards key on their own fields only', () => {
    const draft = conversationDraftOf(withRules)
    expect(draft.choices.map((row) => row.points)).toEqual(['10', '30', '50'])
    expect(draft.rules.map(({ emoji, points, valueKey }) => [emoji, points, valueKey])).toEqual([
      ['pb-10', '10', ''],
      ['pb-30', '30', 'thanks'],
    ])
    expect(validateConversation(draft, program)).toEqual({})
    expect(conversationPatchOf(draft, withRules)).toEqual({})
    expect(deliveryKey(withRules)).toBe(deliveryKey(stored))
    expect(conversationKey(withRules)).not.toBe(conversationKey(stored))
    expect(conversationKey({ ...stored, locale: 'en' })).toBe(conversationKey(stored))
  })

  test('adding, editing and removing rules sends the whole list, emoji normalized and no value as null', () => {
    const rowId = newRowId('rule')
    const added = apply(withRules, { type: 'addRule', rowId }, { type: 'setRule', rowId, change: { emoji: ':Tada::skin-tone-3:', points: '50', valueKey: 'teamwork' } })
    expect(conversationPatchOf(added, withRules)).toEqual({
      reactionRules: [
        { emoji: 'pb-10', points: 10, valueKey: null },
        { emoji: 'pb-30', points: 30, valueKey: 'thanks' },
        { emoji: 'tada', points: 50, valueKey: 'teamwork' },
      ],
    })
    const first = added.rules[0]!.rowId
    const edited = conversationReducer(conversationReducer(added, { type: 'setRule', rowId: first, change: { points: '20' } }), { type: 'removeRule', rowId: added.rules[1]!.rowId })
    expect(conversationPatchOf(edited, withRules).reactionRules).toEqual([
      { emoji: 'pb-10', points: 20, valueKey: null },
      { emoji: 'tada', points: 50, valueKey: 'teamwork' },
    ])
    // Typing ":pb-10:" for a stored "pb-10" is no change; removing every rule sends an empty list.
    const same = conversationDraftOf(withRules)
    expect(conversationPatchOf(conversationReducer(same, { type: 'setRule', rowId: same.rules[0]!.rowId, change: { emoji: ':pb-10:' } }), withRules)).toEqual({})
    const cleared = same.rules.reduce((current, row) => conversationReducer(current, { type: 'removeRule', rowId: row.rowId }), same)
    expect(conversationPatchOf(cleared, withRules)).toEqual({ reactionRules: [] })
    expect(conversationReducer(cleared, { type: 'reset', settings: withRules }).rules.map((row) => row.emoji)).toEqual(['pb-10', 'pb-30'])
  })

  test('point choices: 1–5, the last one stays, the list is sent whole in order', () => {
    const draft = conversationDraftOf(stored)
    const removed = draft.choices.reduce((current, row) => conversationReducer(current, { type: 'removeChoice', rowId: row.rowId }), draft)
    expect(removed.choices.map((row) => row.points)).toEqual(['50'])
    const full = [1, 2, 3, 4].reduce((current) => conversationReducer(current, { type: 'addChoice', rowId: newRowId('choice') }), draft)
    expect(full.choices).toHaveLength(5)
    const rowId = newRowId('choice')
    const grown = conversationReducer(draft, { type: 'addChoice', rowId })
    const typed = conversationReducer(grown, { type: 'setChoice', rowId, points: '5' })
    expect(conversationPatchOf(typed, stored)).toEqual({ mentionPointChoices: [10, 30, 50, 5] })
    expect(conversationPatchOf(removed, stored)).toEqual({ mentionPointChoices: [50] })
  })

  test('problems: amounts out of range or repeated, emoji missing / malformed / repeated, points over the cap, a removed value', () => {
    const draft = conversationDraftOf(withRules)
    const [ten, , fifty] = draft.choices.map((row) => row.rowId)
    const [pb10, pb30] = draft.rules.map((row) => row.rowId)
    const broken = [
      { type: 'setChoice', rowId: ten!, points: '0' },
      { type: 'setChoice', rowId: fifty!, points: '30' },
      { type: 'setRule', rowId: pb10!, change: { emoji: ':PB-30:', points: '121' } },
      { type: 'setRule', rowId: pb30!, change: { valueKey: 'gone' } },
    ] satisfies ConversationAction[]
    const problems = validateConversation(broken.reduce(conversationReducer, draft), program)
    expect(problems).toEqual({
      choices: { [ten!]: 'range', [fifty!]: 'duplicate' },
      rules: { [pb10!]: { points: 'range' }, [pb30!]: { emoji: 'duplicate', valueKey: 'unknown' } },
    })
    const rowId = newRowId('rule')
    const blank = conversationReducer(draft, { type: 'addRule', rowId })
    expect(validateConversation(blank, program).rules?.[rowId]).toEqual({ emoji: 'required', points: 'range' })
    const spaced = conversationReducer(blank, { type: 'setRule', rowId, change: { emoji: 'party parrot', points: '1.5' } })
    expect(validateConversation(spaced, program).rules?.[rowId]).toEqual({ emoji: 'invalid', points: 'range' })
  })

  test('on without the signing secret is a problem; turning it off never is', () => {
    const on = conversationReducer(conversationDraftOf(stored), { type: 'toggle', value: true })
    expect(validateConversation(on, { ...program, signingSecret: false })).toEqual({ giveInConversation: 'signingSecretMissing' })
    expect(validateConversation(on, program)).toEqual({})
    expect(conversationPatchOf(on, stored)).toEqual({ giveInConversation: true })
    const off = conversationReducer(conversationDraftOf({ ...stored, giveInConversation: true }), { type: 'toggle', value: false })
    expect(validateConversation(off, { ...program, signingSecret: false })).toEqual({})
  })

  test('emoji names: colons and skin tones go; everyday emoji are flagged', () => {
    expect(['pb-10', ':pb-10:', ' :PB-10: ', '+1::skin-tone-3', ':+1::skin-tone-3:'].map(normalizeSlackEmoji)).toEqual(['pb-10', 'pb-10', 'pb-10', '+1', '+1'])
    expect(['+1', ':thumbsup:', 'heart', 'pray::skin-tone-2', 'tada'].every(isEverydayEmoji)).toBe(true)
    expect(['pb-10', 'kansha', 'constructor', ''].some(isEverydayEmoji)).toBe(false)
  })
})
