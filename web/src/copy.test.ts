import { createDashboardTranslator } from '@appthrust/platform-dashboard-i18n'
import { describe, expect, test } from 'bun:test'
import { catalogs } from './copy'

const placeholders = (text: string) => [...text.matchAll(/\{([A-Za-z][A-Za-z0-9_]*)\}/g)].map((match) => match[1]).sort()

describe('peerbonus copy catalogs', () => {
  const messages = catalogs.flatMap((catalog) => Object.entries(catalog))

  test('every message is namespaced, translated to Japanese and keeps its placeholders', () => {
    for (const [id, message] of messages) {
      expect(id.startsWith('peerbonus.')).toBe(true)
      const ja = message.translations.ja
      expect(ja, id).toBeTruthy()
      expect(placeholders(ja ?? ''), id).toEqual(placeholders(message.defaultMessage))
    }
  })

  test('ids are unique, so both languages build a translator', () => {
    expect(new Set(messages.map(([id]) => id)).size).toBe(messages.length)
    const ja = createDashboardTranslator('ja', catalogs)
    const en = createDashboardTranslator('en', catalogs)
    expect(ja.t('peerbonus.product')).toBe('ピアボーナス')
    expect(en.t('peerbonus.product')).toBe('Peer Bonus')
    expect(ja.t('peerbonus.nav.balance', { remaining: 120 })).toBe('今週あと 120 pt')
    expect(ja.t('peerbonus.compose.submit', { points: 30 })).toBe('30 pt 贈る')
    expect(en.t('peerbonus.errors.insufficientPoints', { remaining: 4, required: 30 })).toBe('Not enough points left this week: 4 pt left, 30 pt needed.')
  })
})
