import { beforeEach, describe, expect, test } from 'bun:test'
import { peerbonusRoutes } from '../../dev/mock-peerbonus'
import { mockMe, mockPosts, render } from '../../test/render'
import { clapAvailability, ClapButton } from './clap-button'

beforeEach(async () => {
  const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/reset')
  await peerbonusRoutes(new Request(url, { method: 'POST' }), url)
})


describe('clap availability', () => {
  test('never on a post the viewer sent or received or of a closed month; otherwise only with the cost left', () => {
    const post = { sender: { id: 's' }, recipient: { id: 'r' } }
    expect(clapAvailability(post, { memberId: 's', remaining: 400, cost: 2 })).toBe('own')
    expect(clapAvailability(post, { memberId: 'r', remaining: 400, cost: 2, closed: true })).toBe('own')
    expect(clapAvailability(post, { memberId: 'c', remaining: 400, cost: 2, closed: true })).toBe('closed')
    expect(clapAvailability(post, { memberId: 'c', remaining: 1, cost: 2 })).toBe('noBalance')
    expect(clapAvailability(post, { memberId: 'c', remaining: 2, cost: 2 })).toBe('ready')
  })
})

describe('clap button states', () => {
  test("someone else's post: enabled, with the count and the cost described", async () => {
    const me = await mockMe()
    const post = (await mockPosts()).find((item) => item.sender.id !== me.member.id && item.recipient.id !== me.member.id)!
    const html = render(<ClapButton post={{ ...post, clapCount: 5, clappedByMe: 0 }} />, me)
    expect(html).not.toContain('disabled=""')
    expect(html).toContain('拍手<span class="tabular-nums" aria-live="polite">5</span>')
    expect(html).toContain('この投稿に拍手する（2 pt。贈った人ともらった人に +1 ずつ）')
    expect(html).not.toContain('あなた ×')
  })

  test('claps of the caller show as "あなた ×n" and highlight the button', async () => {
    const me = await mockMe()
    const post = (await mockPosts()).find((item) => item.sender.id !== me.member.id && item.recipient.id !== me.member.id)!
    const html = render(<ClapButton post={{ ...post, clapCount: 4, clappedByMe: 2 }} />, me)
    expect(html).toContain('あなた ×2')
    expect(html).toContain('bg-accent-soft')
    expect(render(<ClapButton post={{ ...post, clapCount: 4, clappedByMe: 2 }} />, me, 'en')).toContain('You ×2')
  })

  test('own post and an empty balance disable the button and say why', async () => {
    const me = await mockMe()
    const posts = await mockPosts()
    const own = posts.find((item) => item.sender.id === me.member.id || item.recipient.id === me.member.id)!
    const ownHtml = render(<ClapButton post={own} />, me)
    expect(ownHtml).toContain('disabled=""')
    expect(ownHtml).toContain('自分が関わる投稿には拍手できません')
    const other = posts.find((item) => item.sender.id !== me.member.id && item.recipient.id !== me.member.id)!
    const broke = { ...me, stats: { ...me.stats, period: { ...me.stats.period, remaining: 1 } } }
    const brokeHtml = render(<ClapButton post={other} />, broke)
    expect(brokeHtml).toContain('disabled=""')
    expect(brokeHtml).toContain('今週の残りポイントが足りません')
  })
})
