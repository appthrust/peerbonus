import { describe, expect, test } from 'bun:test'
import { splitLinks } from './message-text'

describe('post message links', () => {
  test('only http(s) URLs become links; sentence punctuation and brackets stay text', () => {
    expect(splitLinks('資料: https://example.com/a?b=1。ありがとう')).toEqual([
      { kind: 'text', text: '資料: ' },
      { kind: 'link', href: 'https://example.com/a?b=1' },
      { kind: 'text', text: '。ありがとう' },
    ])
    expect(splitLinks('see (http://x.test/path).')).toEqual([
      { kind: 'text', text: 'see (' },
      { kind: 'link', href: 'http://x.test/path' },
      { kind: 'text', text: ').' },
    ])
    expect(splitLinks('javascript:alert(1) and ftp://x')).toEqual([{ kind: 'text', text: 'javascript:alert(1) and ftp://x' }])
    expect(splitLinks('')).toEqual([])
  })
})
