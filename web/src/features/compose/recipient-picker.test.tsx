import { beforeEach, describe, expect, test } from 'bun:test'
import type { QueryClient } from '@tanstack/react-query'
import type { Candidate, Recipients } from '../../api/peerbonus'
import { postQuery, recipientsQuery } from '../../api/queries'
import { MOCK_GROUPS, MOCK_PERSON_IDS, peerbonusRoutes } from '../../dev/mock-peerbonus'
import { mockMe, mockPosts, mockRecipients, render } from '../../test/render'
import { ComposeForm } from './compose-form'
import { RecipientPicker } from './recipient-picker'
import { filterCandidates, findRecipient, mergeCandidates, moveSpatial, pickedFrom, pickerPeople } from './recipients'

async function mock(path: string, body?: unknown) {
  const url = new URL(`http://peerbonus.test/api/peerbonus/v1${path}`)
  await peerbonusRoutes(new Request(url, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) }), url)
}

beforeEach(() => mock('/__mock/reset'))

const withRecipients = (recipients: Recipients) => (client: QueryClient) => client.setQueryData(recipientsQuery.queryKey, recipients)
const person = (name: string, extra: Partial<Candidate> = {}): Candidate => ({
  personId: `p-${name}`,
  displayName: name,
  email: `${name.toLowerCase().replace(/\s+/g, '.')}@example.com`,
  pending: false,
  groups: [],
  avatarUrl: null,
  ...extra,
})
/** The markup from the first `start` up to the next `end` after it. */
const between = (html: string, start: string, end: string) => html.slice(html.indexOf(start), html.indexOf(end, html.indexOf(start)))

describe('recipient picker (static)', () => {
  test('search first; then recent chips and each team as cards, everyone behind a disclosure; each person once, never the caller', async () => {
    const me = await mockMe()
    const recipients = await mockRecipients()
    const html = render(<RecipientPicker id="r" value={null} onChange={() => {}} />, me, 'ja', withRecipients(recipients))
    expect(html).toContain('placeholder="名前やメールで検索"')
    expect(html.indexOf('type="search"')).toBeLessThan(html.indexOf('>最近</h3>'))
    expect(html.indexOf('>最近</h3>')).toBeLessThan(html.indexOf('>チーム</h3>'))
    for (const recent of recipients.recent) expect(html).toContain(`data-candidate="${recent.personId}"`)
    // 佐藤 is recent and in team-a: shown once, as a recent chip.
    expect(html.match(new RegExp(`data-candidate="${MOCK_PERSON_IDS.sato}"`, 'g'))).toHaveLength(1)
    expect(html.indexOf('>sales/team-a</h4>')).toBeLessThan(html.indexOf('>projects/ceatec</h4>'))
    const teamA = between(html, '>sales/team-a</h4>', '</ul>')
    expect(teamA).toContain('aria-label="sales/team-a のメンバー"')
    expect(teamA).toContain('田中 美咲')
    expect(teamA).not.toContain('佐藤 花子')
    expect(teamA).not.toContain('中村 翔')
    // 鈴木 never signed in: the same card with a quiet clock, no badge.
    const suzuki = between(teamA, 'data-candidate="p-suzuki"', '</button>')
    expect(suzuki).toContain('<span class="sr-only">未ログイン</span>')
    expect(suzuki).not.toContain('border-dashed')
    expect(suzuki).not.toContain('rounded bg-stone-100')
    expect(suzuki).toMatch(/title="suzuki@example.com"/)
    const ceatec = between(html, '>projects/ceatec</h4>', '</ul>')
    expect(ceatec).toContain('伊藤 さくら')
    expect(ceatec).toContain('中村 翔')
    expect(html).toContain('全員を見る（8）')
    expect(html).not.toContain('>全員</h3>')
    expect(html).not.toContain('山田 太郎')
    expect(html).not.toContain(`data-candidate="${MOCK_PERSON_IDS.yamada}"`)
    // Every person is one tab stop, on the first.
    expect(html.match(/tabindex="0"/g)).toHaveLength(1)
    expect(html).toMatch(new RegExp(`data-candidate="${recipients.recent[0]!.personId}" tabindex="0"`))
  })

  test('no team: everyone is open in name order, without the people already shown; English copy too', async () => {
    const me = await mockMe()
    const recipients = await mockRecipients()
    const html = render(<RecipientPicker id="r" value={null} onChange={() => {}} />, me, 'ja', withRecipients({ ...recipients, teams: [] }))
    expect(html).not.toContain('全員を見る')
    expect(html).toContain('>全員</h3>')
    const list = between(html, 'aria-label="全員（名前順）"', '</ul>')
    const order = [...list.matchAll(/data-candidate="([^"]+)"/g)].map((match) => match[1])
    const recent = new Set(recipients.recent.map((item) => item.personId))
    expect(order).toEqual(recipients.everyone.items.map((item) => item.personId).filter((id) => !recent.has(id)))
    const en = render(<RecipientPicker id="r" value={null} onChange={() => {}} />, me, 'en', withRecipients(recipients))
    expect(en).toContain('placeholder="Search by name or email"')
    expect(en).toContain('>Recent</h3>')
    expect(en).toContain('See everyone (8)')
  })

  test('teams with no one left (empty, or everyone already recent) are left out', async () => {
    const me = await mockMe()
    const recipients = await mockRecipients()
    const empty = { group: { id: 'g-empty', name: '空', path: 'empty/team' }, members: [] }
    const onlyRecent = { group: { id: 'g-r', name: 'R', path: 'recent/only' }, members: recipients.recent }
    const html = render(<RecipientPicker id="r" value={null} onChange={() => {}} />, me, 'ja', withRecipients({ ...recipients, teams: [...recipients.teams, empty, onlyRecent] }))
    expect(html).toContain('>projects/ceatec</h4>')
    expect(html).not.toContain('empty/team')
    expect(html).not.toContain('recent/only')
  })

  test('Directory down: the note, the search field and the recent people only', async () => {
    const me = await mockMe()
    await mock('/__mock/directory', { up: false })
    const recipients = await mockRecipients()
    expect(recipients.directoryAvailable).toBe(false)
    const html = render(<RecipientPicker id="r" value={null} onChange={() => {}} />, me, 'ja', withRecipients(recipients))
    expect(html).toContain('Directory に接続できません。既に贈ったことのある人には贈れます')
    expect(html).toContain('type="search"')
    expect(html).toContain('>最近</h3>')
    expect(html).not.toContain('>チーム</h3>')
    expect(html).not.toContain('全員を見る')
  })

  test('Directory holding more people than loaded says so under everyone', async () => {
    const me = await mockMe()
    const items = [person('Ada Lovelace'), person('Grace Hopper', { pending: true })]
    const html = render(
      <RecipientPicker id="r" value={null} onChange={() => {}} />,
      me,
      'ja',
      withRecipients({ me: { personId: me.member.personId, groups: [] }, recent: [], teams: [], everyone: { total: 140, items }, directoryAvailable: true })
    )
    expect(html).toContain('140 人中 2 人を表示しています。')
    expect(html).toContain('>AL<')
  })

  test('loading shows placeholder cards under the search field', async () => {
    const html = render(<RecipientPicker id="r" value={null} onChange={() => {}} />, await mockMe())
    expect(html).toContain('aria-busy="true"')
    expect(html).toContain('贈れる相手を読み込んでいます…')
    expect(html).toContain('type="search"')
  })

  test('a chosen person is a chip with the group, email and 変更', async () => {
    const me = await mockMe()
    const sato = person('佐藤 花子', { email: 'sato@example.com', groups: [MOCK_GROUPS.teamA] })
    const html = render(<RecipientPicker id="r" value={sato} onChange={() => {}} />, me)
    expect(html).toContain('佐藤 花子')
    expect(html).toContain('sales/team-a · sato@example.com')
    expect(html).toContain('aria-label="宛先（佐藤 花子 さん）を変更"')
    expect(html).toContain('>変更<')
    expect(html).not.toContain('data-candidate')
    expect(html).not.toContain('type="search"')
  })
})

describe('compose form with a recipient', () => {
  test('?to= picks the person: chip, and the button names them', async () => {
    const me = await mockMe()
    const recipients = await mockRecipients()
    const html = render(<ComposeForm to={MOCK_PERSON_IDS.sato} />, me, 'ja', withRecipients(recipients))
    expect(html).toContain('aria-label="宛先（佐藤 花子 さん）を変更"')
    expect(html).toContain('sales/team-a')
    expect(html).toContain('佐藤 花子 さんに 30 pt 贈る')
    expect(html).not.toContain('初回サインイン後に本人へ届きます')
    expect(render(<ComposeForm to={MOCK_PERSON_IDS.sato} />, me, 'en', withRecipients(recipients))).toContain('Send 30 pt to 佐藤 花子')
  })

  test('a pending recipient gets the first-sign-in note', async () => {
    const me = await mockMe()
    const html = render(<ComposeForm to={MOCK_PERSON_IDS.suzuki} />, me, 'ja', withRecipients(await mockRecipients()))
    expect(html).toContain('未ログイン')
    expect(html).toContain('初回サインイン後に本人へ届きます（Directory の email と一致が必要）')
    expect(html).toContain('鈴木 一郎 さんに 30 pt 贈る')
  })

  test('?to= falls back to a Member a loaded post showed; the caller and unknown people pick no one', async () => {
    const me = await mockMe()
    await mock('/__mock/directory', { up: false })
    const down = await mockRecipients()
    const post = (await mockPosts()).find((item) => item.recipient.id !== me.member.id && !down.recent.some((recent) => recent.personId === item.recipient.personId))!
    expect(post).toBeDefined()
    const seed = (client: QueryClient) => {
      withRecipients(down)(client)
      client.setQueryData(postQuery(post.id).queryKey, post)
    }
    expect(render(<ComposeForm to={post.recipient.personId} />, me, 'ja', seed)).toContain(`${post.recipient.displayName} さんに 30 pt 贈る`)
    for (const to of [me.member.personId, 'p-nobody']) {
      const html = render(<ComposeForm to={to} />, me, 'ja', seed)
      expect(html).toContain('>30 pt 贈る<')
      expect(html).not.toContain('を変更"')
    }
  })
})

describe('recipient model', () => {
  const people = [person('佐藤 花子', { email: 'hanako@example.com' }), person('Ken Watanabe'), person('ｻﾄｳ ｼﾞﾛｳ', { email: 'jiro@corp.example' })]

  test('the filter narrows by name or email, ignoring case, width and spaces', () => {
    expect(filterCandidates(people, '').map((item) => item.displayName)).toEqual(people.map((item) => item.displayName))
    expect(filterCandidates(people, '佐藤花子').map((item) => item.displayName)).toEqual(['佐藤 花子'])
    expect(filterCandidates(people, 'ken w').map((item) => item.displayName)).toEqual(['Ken Watanabe'])
    expect(filterCandidates(people, 'サトウ').map((item) => item.displayName)).toEqual(['ｻﾄｳ ｼﾞﾛｳ'])
    expect(filterCandidates(people, 'HANAKO@').map((item) => item.displayName)).toEqual(['佐藤 花子'])
    expect(filterCandidates(people, 'nobody')).toEqual([])
  })

  test('server results add only people not loaded yet', () => {
    const extra = person('Zed')
    expect(mergeCandidates(people.slice(0, 1), [people[0]!, extra]).map((item) => item.personId)).toEqual([people[0]!.personId, extra.personId])
  })

  test('picking from a team puts that team first', () => {
    const ito = person('伊藤', { groups: [MOCK_GROUPS.teamB, MOCK_GROUPS.ceatec] })
    expect(pickedFrom(ito, MOCK_GROUPS.ceatec).groups).toEqual([MOCK_GROUPS.ceatec, MOCK_GROUPS.teamB])
  })

  test('sections hold each person once (recent > team > everyone), narrowed by the query, with server finds last', () => {
    const [a, b, c, d] = ['A', 'B', 'C', 'D'].map((name) => person(name))
    const recipients: Recipients = {
      me: { personId: 'p-me', groups: [] },
      recent: [a!],
      teams: [
        { group: MOCK_GROUPS.teamA, members: [a!, b!] },
        { group: MOCK_GROUPS.ceatec, members: [b!, c!] },
      ],
      everyone: { total: 4, items: [a!, b!, c!, d!] },
      directoryAvailable: true,
    }
    const people = pickerPeople(recipients, '')
    expect(people.recent.map((item) => item.personId)).toEqual(['p-A'])
    expect(people.teams.map((team) => [team.group.path, team.members.map((item) => item.personId)])).toEqual([
      ['sales/team-a', ['p-B']],
      ['projects/ceatec', ['p-C']],
    ])
    expect(people.teams[0]!.members[0]!.groups[0]).toEqual(MOCK_GROUPS.teamA)
    expect(people.everyone.map((item) => item.personId)).toEqual(['p-D'])
    const searched = pickerPeople(recipients, 'c@', [c!, person('Cy', { email: 'c@other.test' })])
    expect(searched.recent).toEqual([])
    expect(searched.teams.map((team) => team.members.map((item) => item.personId))).toEqual([['p-C']])
    expect(searched.everyone.map((item) => item.personId)).toEqual(['p-Cy'])
    // Directory down: only the recent people and the server's finds.
    const down = pickerPeople({ ...recipients, directoryAvailable: false }, '', [d!])
    expect(down.teams).toEqual([])
    expect(down.everyone.map((item) => item.personId)).toEqual(['p-D'])
  })

  test('roving focus follows the layout: left/right in order, up/down to the nearest row and column, edges stay', () => {
    // A row of two chips (y 0), then cards in two columns (y 40, 90): 0 1 / 2 3 / 4
    const spots = [
      { x: 0, y: 0 },
      { x: 90, y: 0 },
      { x: 0, y: 40 },
      { x: 200, y: 42 },
      { x: 0, y: 90 },
    ]
    expect(moveSpatial('ArrowRight', 1, spots)).toBe(2)
    expect(moveSpatial('ArrowRight', 4, spots)).toBe(4)
    expect(moveSpatial('ArrowLeft', 0, spots)).toBe(0)
    expect(moveSpatial('ArrowDown', 1, spots)).toBe(2)
    expect(moveSpatial('ArrowDown', 3, spots)).toBe(4)
    expect(moveSpatial('ArrowDown', 4, spots)).toBe(4)
    expect(moveSpatial('ArrowUp', 3, spots)).toBe(1)
    expect(moveSpatial('ArrowUp', 0, spots)).toBe(0)
    expect(moveSpatial('Home', 3, spots)).toBe(0)
    expect(moveSpatial('End', 0, spots)).toBe(4)
    expect(moveSpatial('a', 0, spots)).toBeUndefined()
  })

  test('?to= resolves from the picker people first, never to the caller', () => {
    const recipients: Recipients = {
      me: { personId: 'p-me', groups: [] },
      recent: [],
      teams: [{ group: MOCK_GROUPS.teamA, members: [person('A', { groups: [MOCK_GROUPS.teamA] })] }],
      everyone: { total: 1, items: [person('A')] },
      directoryAvailable: true,
    }
    expect(findRecipient('p-A', recipients, undefined, 'p-me')?.groups).toEqual([MOCK_GROUPS.teamA])
    const avatarUrl = '/api/peerbonus/v1/avatars/p-B?v=2026-10-01T00:00:00Z'
    const member = { id: 'm', personId: 'p-B', displayName: 'B', email: 'b@example.com', pending: true, directoryUrl: null, avatarUrl }
    expect(findRecipient('p-B', recipients, member, 'p-me')).toEqual({ personId: 'p-B', displayName: 'B', email: 'b@example.com', pending: true, groups: [], avatarUrl })
    expect(findRecipient('p-me', recipients, { ...member, personId: 'p-me' }, 'p-me')).toBeUndefined()
    expect(findRecipient('p-C', undefined, undefined, 'p-me')).toBeUndefined()
  })
})
