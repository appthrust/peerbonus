import { describe, expect, test } from 'bun:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { Avatar, AvatarStack } from './misc'

const url = '/api/peerbonus/v1/avatars/p-sato?v=2026-10-01T00:00:00Z'

describe('avatar', () => {
  test('renders the relayed picture when avatarUrl is set, the initials when it is null', () => {
    const picture = renderToStaticMarkup(<Avatar name="Ada Lovelace" avatarUrl={url} size="lg" />)
    expect(picture).toStartWith('<img')
    expect(picture).toContain(`src="${url}"`)
    expect(picture).toContain('alt=""')
    expect(picture).toContain('aria-hidden="true"')
    expect(picture).toContain('loading="lazy"')
    expect(picture).not.toContain('AL')

    const initials = renderToStaticMarkup(<Avatar name="Ada Lovelace" avatarUrl={null} />)
    expect(initials).not.toContain('<img')
    expect(initials).toContain('>AL</span>')
    expect(renderToStaticMarkup(<Avatar name="山田 太郎" />)).toContain('>山</span>')
  })

  test('a stack shows each member by picture or initial', () => {
    const html = renderToStaticMarkup(
      <AvatarStack
        members={[
          { displayName: '佐藤 花子', avatarUrl: url },
          { displayName: '鈴木 一郎', avatarUrl: null },
        ]}
        total={5}
      />
    )
    expect(html).toContain(`src="${url}"`)
    expect(html).toContain('>鈴</span>')
    expect(html).toContain('+3')
  })
})
