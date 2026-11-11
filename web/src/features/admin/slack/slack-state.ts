// What `/admin/slack` shows for the server's Slack read model: the state chip,
// the one action the status card offers, how far the setup guide got, and the
// labels of deliveries and Slack error codes. Pure, so the mapping is tested
// without rendering.
import type { SlackAdmin, SlackDeliveryKind, SlackDeliveryState, SlackState } from '../../../api/peerbonus'
import type { MessageId } from '../../../copy'

export type Tone = 'ok' | 'busy' | 'bad' | 'neutral'

/**
 * The status card's action: none (no token yet), `verify` (first fix of the workspace), `reverify`
 * (refresh the name, same workspace), `force` (re-fix to the token's workspace after a confirm).
 */
export type VerifyAction = 'none' | 'verify' | 'reverify' | 'force'

export type StateView = { label: MessageId; body: MessageId; tone: Tone; action: VerifyAction }

/** The chip, explanation and action for each server state. */
export const STATE_VIEWS: Record<SlackState, StateView> = {
  unconfigured: { label: 'peerbonus.slack.state.unconfigured', body: 'peerbonus.slack.state.unconfiguredBody', tone: 'neutral', action: 'none' },
  configured: { label: 'peerbonus.slack.state.configured', body: 'peerbonus.slack.state.configuredBody', tone: 'busy', action: 'verify' },
  connected: { label: 'peerbonus.slack.state.connected', body: 'peerbonus.slack.state.connectedBody', tone: 'ok', action: 'reverify' },
  team_changed: { label: 'peerbonus.slack.state.teamChanged', body: 'peerbonus.slack.state.teamChangedBody', tone: 'bad', action: 'force' },
}

export type SetupStepId = 'createApp' | 'pasteToken' | 'connect' | 'signingSecret'
export type SetupStep = { id: SetupStepId; done: boolean }

/**
 * The setup guide's numbered steps (design §8) with what the read model proves done. Creating the app
 * and pasting its token both show as done once the token is there. The signing secret step (Phase B)
 * appears only once the platform projected the inbound endpoint.
 */
export function setupSteps(slack: Pick<SlackAdmin, 'credential' | 'state' | 'settings' | 'inbound'>): SetupStep[] {
  const steps: SetupStep[] = [
    { id: 'createApp', done: slack.credential.botToken },
    { id: 'pasteToken', done: slack.credential.botToken },
    { id: 'connect', done: slack.state === 'connected' && slack.settings.announceChannel !== null },
  ]
  if (slack.inbound) steps.push({ id: 'signingSecret', done: slack.credential.signingSecret && slack.settings.giveFromSlack })
  return steps
}

/**
 * Why giving in a conversation cannot be switched on, with its explanation: the server refuses it without
 * the signing secret, and without the inbound URL Slack has nowhere to send mentions and reactions.
 * Null when nothing is missing.
 */
export function conversationBlocker(slack: Pick<SlackAdmin, 'credential' | 'inbound'>): MessageId | null {
  if (!slack.credential.signingSecret) return 'peerbonus.slack.giveFromSlackLocked'
  if (!slack.inbound?.url) return 'peerbonus.slack.conversation.noInbound'
  return null
}

export const DELIVERY_STATES: readonly SlackDeliveryState[] = ['pending', 'sent', 'skipped', 'failed']

export const DELIVERY_STATE_VIEWS: Record<SlackDeliveryState, { label: MessageId; tone: Tone }> = {
  pending: { label: 'peerbonus.slack.deliveries.state.pending', tone: 'busy' },
  sent: { label: 'peerbonus.slack.deliveries.state.sent', tone: 'ok' },
  skipped: { label: 'peerbonus.slack.deliveries.state.skipped', tone: 'neutral' },
  failed: { label: 'peerbonus.slack.deliveries.state.failed', tone: 'bad' },
}

export const DELIVERY_KIND_LABELS: Record<SlackDeliveryKind, MessageId> = {
  post_announce: 'peerbonus.slack.deliveries.kind.postAnnounce',
  post_dm: 'peerbonus.slack.deliveries.kind.postDm',
  clap_update: 'peerbonus.slack.deliveries.kind.clapUpdate',
  post_delete: 'peerbonus.slack.deliveries.kind.postDelete',
}

/** Slack error codes and skip reasons with an explanation; any other code shows as itself. */
export const SLACK_ERROR_LABELS: Partial<Record<string, MessageId>> = {
  team_changed: 'peerbonus.slack.error.teamChanged',
  invalid_auth: 'peerbonus.slack.error.invalidAuth',
  token_revoked: 'peerbonus.slack.error.tokenRevoked',
  account_inactive: 'peerbonus.slack.error.tokenRevoked',
  not_in_channel: 'peerbonus.slack.error.notInChannel',
  channel_not_found: 'peerbonus.slack.error.channelNotFound',
  is_archived: 'peerbonus.slack.error.channelNotFound',
  missing_scope: 'peerbonus.slack.error.missingScope',
  ratelimited: 'peerbonus.slack.error.rateLimited',
  user_not_found: 'peerbonus.slack.error.userNotFound',
  announce_not_sent: 'peerbonus.slack.error.announceNotSent',
  post_deleted: 'peerbonus.slack.error.postDeleted',
}
