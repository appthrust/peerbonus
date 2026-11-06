import { describe, expect, test } from 'bun:test'
import { generateManifest, manifestHref, manifestJson } from './manifest'

const INBOUND = 'https://in.appthrust.ink/pj-1/peerbonus-slack'

describe('Slack app manifest (design §8)', () => {
  test('Phase A: the bot and its output scopes only, no command or interactivity', () => {
    expect(generateManifest({ inboundUrl: null, phaseB: false })).toEqual({
      display_information: { name: 'Peer Bonus', description: 'Send points and thanks to colleagues from Slack.' },
      features: { bot_user: { display_name: 'Peer Bonus', always_online: true } },
      oauth_config: { scopes: { bot: ['chat:write', 'channels:read', 'users:read', 'users:read.email', 'im:write'] } },
      settings: { org_deploy_enabled: false, socket_mode_enabled: false },
    })
    // A known inbound URL does not leak into a Phase A manifest.
    expect(generateManifest({ inboundUrl: INBOUND, phaseB: false })).toEqual(generateManifest({ inboundUrl: null, phaseB: false }))
  })

  test('Phase B: /peerbonus, interactivity and options load URL under the inbound endpoint, plus the commands scope', () => {
    expect(generateManifest({ inboundUrl: `${INBOUND}/`, phaseB: true })).toEqual({
      display_information: { name: 'Peer Bonus', description: 'Send points and thanks to colleagues from Slack.' },
      features: {
        bot_user: { display_name: 'Peer Bonus', always_online: true },
        slash_commands: [{ command: '/peerbonus', url: `${INBOUND}/commands`, description: 'Send points and thanks', usage_hint: '[balance]' }],
      },
      oauth_config: { scopes: { bot: ['chat:write', 'channels:read', 'users:read', 'users:read.email', 'im:write', 'commands'] } },
      settings: {
        interactivity: { is_enabled: true, request_url: `${INBOUND}/interactions`, message_menu_options_url: `${INBOUND}/options` },
        org_deploy_enabled: false,
        socket_mode_enabled: false,
      },
    })
    expect(() => generateManifest({ inboundUrl: null, phaseB: true })).toThrow()
  })

  test('each call builds a fresh object; the download is the pretty JSON', () => {
    const first = generateManifest({ inboundUrl: INBOUND, phaseB: true })
    first.oauth_config.scopes.bot.push('admin')
    expect(generateManifest({ inboundUrl: null, phaseB: false }).oauth_config.scopes.bot).not.toContain('admin')
    const manifest = generateManifest({ inboundUrl: null, phaseB: false })
    const text = manifestJson(manifest)
    expect(JSON.parse(text)).toEqual(manifest)
    expect(text.endsWith('}\n')).toBe(true)
    expect(text).toContain('\n  "display_information": {')
    expect(decodeURIComponent(manifestHref(manifest).replace('data:application/json;charset=utf-8,', ''))).toBe(text)
  })
})
