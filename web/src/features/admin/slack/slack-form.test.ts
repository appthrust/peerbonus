import { describe, expect, test } from 'bun:test'
import type { SlackSettings } from '../../../api/peerbonus'
import { slackDraftReducer, slackPatchOf, validateSlackDraft, type SlackDraftAction } from './slack-form'

const stored: SlackSettings = { announceChannel: null, announcePosts: false, dmRecipientOnPost: true, giveFromSlack: false, locale: 'ja' }
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
