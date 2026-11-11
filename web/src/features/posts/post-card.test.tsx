import { beforeEach, describe, expect, test } from 'bun:test'
import type { Member } from '../../api/peerbonus'
import { peerbonusRoutes } from '../../dev/mock-peerbonus'
import { mockMe, mockPosts, render } from '../../test/render'
import { PostCard } from './post-card'

beforeEach(async () => {
  const url = new URL('http://peerbonus.test/api/peerbonus/v1/__mock/reset')
  await peerbonusRoutes(new Request(url, { method: 'POST' }), url)
})

const text = (html: string) => html.replace(/<[^>]+>/g, '')

describe('post card', () => {
  test('the recipient is the headline, then the points, the message and "from" the sender', async () => {
    const me = await mockMe()
    const post = (await mockPosts())[0]
    const html = render(<PostCard post={post} />, me)
    const plain = text(html)
    const headline = `${post.recipient.displayName} さんへ`
    expect(plain).toContain(headline)
    expect(plain).toContain(`+${post.points} pt`)
    expect(plain).toContain(`from ${post.sender.displayName}`)
    // Reading order: recipient, points, message, sender.
    const at = (needle: string) => plain.indexOf(needle)
    expect(at(headline)).toBeLessThan(at(`+${post.points} pt`))
    expect(at(`+${post.points} pt`)).toBeLessThan(at(post.message.slice(0, 10)))
    expect(at(post.message.slice(0, 10))).toBeLessThan(at(`from ${post.sender.displayName}`))
    expect(text(render(<PostCard post={post} />, me, 'en'))).toContain(`To ${post.recipient.displayName}`)
  })

  test('values are coloured chips with the Program label; a deleted key reads #key', async () => {
    const me = await mockMe()
    const post = (await mockPosts())[0]
    const value = me.program.values[0]
    const html = render(<PostCard post={{ ...post, values: [value.key, 'retired-value'] }} />, me)
    expect(html).toContain(`>${value.label}</span>`)
    expect(html).toContain('bg-amber-50')
    expect(html).toContain('>#retired-value</span>')
    expect(html).toContain('bg-stone-100')
  })

  test('clappers render as avatars (picture or initials), each person once, with +N for the remaining claps', async () => {
    const me = await mockMe()
    const post = (await mockPosts())[0]
    const picture = '/api/peerbonus/v1/avatars/p-ada?v=2026-10-01T00:00:00Z'
    const people: Member[] = [
      { ...post.sender, id: 'c1', displayName: 'Ada Lovelace', avatarUrl: picture },
      { ...post.sender, id: 'c2', displayName: '山田 花子', avatarUrl: null },
    ]
    const html = render(<PostCard post={{ ...post, clapCount: 6, clappers: [people[0], people[1], people[0]] }} />, me)
    expect(html.split(`src="${picture}"`)).toHaveLength(2)
    expect(html).toContain('>山</span>')
    expect(html).toContain('+4')
    expect(html).toContain('拍手した人: Ada Lovelace、山田 花子 ほか 4 件')
    const none = render(<PostCard post={{ ...post, clapCount: 0, clappers: [] }} />, me)
    expect(none).not.toContain('拍手した人')
  })

  test('highlighted plays the one-shot glow; otherwise no highlight', async () => {
    const me = await mockMe()
    const post = (await mockPosts())[0]
    expect(render(<PostCard post={post} highlighted />, me)).toContain('animate-highlight')
    expect(render(<PostCard post={post} />, me)).not.toContain('animate-highlight')
  })

  test('「同じ人に贈る」 only on posts between two other people; the overflow menu only for admins', async () => {
    const me = await mockMe()
    const posts = await mockPosts()
    const other = posts.find((item) => item.sender.id !== me.member.id && item.recipient.id !== me.member.id)!
    const own = posts.find((item) => item.sender.id === me.member.id)!
    expect(render(<PostCard post={other} />, me)).toContain('同じ人に贈る')
    expect(render(<PostCard post={own} />, me)).not.toContain('同じ人に贈る')
    const admin = { ...me, roles: { peerbonusAdmin: true } }
    const member = { ...me, roles: { peerbonusAdmin: false } }
    expect(render(<PostCard post={other} />, admin)).toContain('aria-label="投稿の操作"')
    expect(render(<PostCard post={other} />, member)).not.toContain('aria-label="投稿の操作"')
  })

  test('in a list the time links to the post; on the post page it is the full timestamp', async () => {
    const me = await mockMe()
    const post = (await mockPosts())[0]
    expect(render(<PostCard post={post} />, me)).toContain(`href="/posts/${post.id}"`)
    const detail = render(<PostCard post={post} detail />, me)
    expect(detail).not.toContain(`href="/posts/${post.id}"`)
    expect(detail).toContain('text-lg')
  })

  test('a post given in Slack links its original message in a new tab; a web post has no such link', async () => {
    const me = await mockMe()
    const post = (await mockPosts())[0]!
    const permalink = 'https://acme.slack.com/archives/C0KUDOS/p1760929200000100'
    const html = render(<PostCard post={{ ...post, slackPermalink: permalink }} />, me)
    expect(html).toContain(`<a href="${permalink}" target="_blank" rel="noreferrer"`)
    expect(text(html)).toContain('Slack の元メッセージ')
    expect(text(render(<PostCard post={{ ...post, slackPermalink: permalink }} />, me, 'en'))).toContain('Original Slack message')
    expect(render(<PostCard post={{ ...post, slackPermalink: null }} />, me)).not.toContain('Slack の元メッセージ')
  })
})
