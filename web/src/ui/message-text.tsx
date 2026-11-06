import { cn } from './cn'

/** Plain text split into text runs and http(s) links (spec §3.2: no Markdown, URLs auto-linked only). */
export type TextPart = { kind: 'text'; text: string } | { kind: 'link'; href: string }

// Stops at whitespace and at closing brackets / CJK punctuation that usually end a sentence, not a URL.
const URL_PATTERN = /https?:\/\/[^\s<>"'()（）「」、。]+/g
// Sentence punctuation right after a URL belongs to the sentence.
const TRAILING = /[.,!?;:！？]+$/

export function splitLinks(text: string): TextPart[] {
  const parts: TextPart[] = []
  let last = 0
  for (const match of text.matchAll(URL_PATTERN)) {
    const href = match[0].replace(TRAILING, '')
    const start = match.index
    if (start > last) parts.push({ kind: 'text', text: text.slice(last, start) })
    parts.push({ kind: 'link', href })
    last = start + href.length
  }
  if (last < text.length) parts.push({ kind: 'text', text: text.slice(last) })
  return parts
}

/** A post message: line breaks kept, URLs as links opening in a new tab. `lg` is the post page's reading size. */
export function MessageText({ text, size = 'md', className }: { text: string; size?: 'md' | 'lg'; className?: string }) {
  return (
    <p className={cn('leading-relaxed break-words whitespace-pre-wrap text-stone-800', size === 'lg' ? 'text-lg sm:text-xl' : 'text-base', className)}>
      {splitLinks(text).map((part, index) =>
        part.kind === 'text' ? (
          part.text
        ) : (
          <a
            key={index}
            href={part.href}
            target="_blank"
            rel="noopener noreferrer nofollow"
            className="relative z-[1] break-all text-accent-strong underline decoration-accent-line underline-offset-2 hover:decoration-accent"
          >
            {part.href}
          </a>
        )
      )}
    </p>
  )
}
