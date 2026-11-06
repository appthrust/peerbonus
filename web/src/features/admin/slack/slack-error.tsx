import { isPeerbonusError, slackErrorDetail } from '../../../api/peerbonus'
import type { MessageId } from '../../../copy'
import { useText } from '../../../i18n'
import { ActionError, Notice } from '../../../ui'
import { SLACK_ERROR_LABELS } from './slack-state'

/** Slack refusals with a fixed explanation (design §5 error codes). */
const CODE_MESSAGES: Partial<Record<string, MessageId>> = {
  slack_not_configured: 'peerbonus.slack.error.notConfigured',
  slack_not_connected: 'peerbonus.slack.error.notConnected',
  slack_signing_secret_missing: 'peerbonus.slack.error.signingSecretMissing',
  slack_team_changed: 'peerbonus.slack.error.teamChanged',
}

/**
 * A failed Slack action, inline where it happened: the bot missing from `channel` names the
 * `/invite` to run; a Slack error shows Slack's own code (explained when known); anything else falls
 * back to the shared ActionError.
 */
export function SlackActionError({ error, channel, className }: { error: unknown; channel?: string; className?: string }) {
  const { t } = useText()
  if (isPeerbonusError(error, 'slack_bot_not_in_channel')) {
    return (
      <Notice tone="error" className={className}>
        {t('peerbonus.slack.error.botNotInChannel', { channel: channel ?? '' })}
      </Notice>
    )
  }
  const detail = slackErrorDetail(error)
  if (detail !== undefined) {
    const label = SLACK_ERROR_LABELS[detail]
    return (
      <Notice tone="error" className={className}>
        {t('peerbonus.slack.error.slack', { code: label ? t(label) : detail })}
      </Notice>
    )
  }
  const known = isPeerbonusError(error) ? CODE_MESSAGES[error.code] : undefined
  if (known) {
    return (
      <Notice tone="error" className={className}>
        {t(known)}
      </Notice>
    )
  }
  return <ActionError error={error} className={className} />
}
