// The Slack app manifest an admin pastes into https://api.slack.com/apps
// (Create New App › From a manifest), design slack-integration §8, as JSON.
// Phase A (output only) has the bot and its scopes; Phase B adds the
// `/peerbonus` command, interactivity and the options load URL, all pointing
// at the Slack inbound endpoint the platform projected for this Project.

/** The bot's display name; the `/invite @Peer Bonus` hint names it too. */
export const SLACK_APP_NAME = 'Peer Bonus'

const PHASE_A_SCOPES = ['chat:write', 'channels:read', 'users:read', 'users:read.email', 'im:write'] as const

export type SlackManifest = {
  display_information: { name: string; description: string }
  features: {
    bot_user: { display_name: string; always_online: boolean }
    slash_commands?: { command: string; url: string; description: string; usage_hint: string }[]
  }
  oauth_config: { scopes: { bot: string[] } }
  settings: {
    interactivity?: { is_enabled: boolean; request_url: string; message_menu_options_url: string }
    org_deploy_enabled: boolean
    socket_mode_enabled: boolean
  }
}

/**
 * The manifest for `phaseB` (Slack → Peer Bonus: command, modal, clap button) or Phase A (announcements
 * and DMs only). `inboundUrl` is the endpoint's base (`https://in.<base>/<project>/<endpoint>`);
 * Phase B needs it, Phase A ignores it.
 */
export function generateManifest({ inboundUrl, phaseB }: { inboundUrl: string | null; phaseB: boolean }): SlackManifest {
  const manifest: SlackManifest = {
    display_information: { name: SLACK_APP_NAME, description: 'Send points and thanks to colleagues from Slack.' },
    features: { bot_user: { display_name: SLACK_APP_NAME, always_online: true } },
    oauth_config: { scopes: { bot: [...PHASE_A_SCOPES] } },
    settings: { org_deploy_enabled: false, socket_mode_enabled: false },
  }
  if (!phaseB) return manifest
  if (!inboundUrl) throw new Error('a Phase B manifest needs the inbound URL')
  const base = inboundUrl.replace(/\/+$/, '')
  manifest.features.slash_commands = [{ command: '/peerbonus', url: `${base}/commands`, description: 'Send points and thanks', usage_hint: '[balance]' }]
  manifest.oauth_config.scopes.bot.push('commands')
  manifest.settings = {
    interactivity: { is_enabled: true, request_url: `${base}/interactions`, message_menu_options_url: `${base}/options` },
    ...manifest.settings,
  }
  return manifest
}

/** Pretty-printed JSON with a trailing newline, as the download holds it. */
export function manifestJson(manifest: SlackManifest): string {
  return `${JSON.stringify(manifest, null, 2)}\n`
}

/** A `data:` URL for an `<a download>` of the manifest (no object URL to revoke). */
export function manifestHref(manifest: SlackManifest): string {
  return `data:application/json;charset=utf-8,${encodeURIComponent(manifestJson(manifest))}`
}
