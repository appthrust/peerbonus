import { ExternalLink, Gift } from 'lucide-react'
import type { ReactNode } from 'react'
import type { Post } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { useSession } from '../../shell/use-session'
import { Avatar, AvatarStack, cn, formatDateTime, formatRelativeTime, HrefLink, LinkButton, MessageText, useNow } from '../../ui'
import { ClapButton } from './clap-button'
import { PostMenu } from './delete-post'
import { ValueTag } from './value-tag'

// Placeholder for a node inside a translated sentence ("{name} さんへ" / "To {name}").
const SLOT = '\u0000'

/** `sentence` (translated with SLOT for the placeholder) with `node` where the placeholder was. */
function Sentence({ sentence, children }: { sentence: string; children: ReactNode }) {
  const [before, after] = sentence.split(SLOT)
  return (
    <>
      {before}
      {children}
      {after}
    </>
  )
}

/**
 * The only post renderer (timeline, profiles, inbox, the post page). The
 * recipient is the headline — "山田 太郎 さんへ" with a big "+30 pt" — then
 * the message, its values, and a footer: the clap button with who clapped,
 * 「同じ人に贈る」, and "from 鈴木 一郎 · 3 時間前" (the time links to the post
 * page), then a link to the Slack message a post given in Slack came from.
 * `detail` is the post page: larger text and the full timestamp.
 * `highlighted` plays a one-shot glow for a post the viewer just sent (the
 * launcher scrolls it into view). Admins get an overflow menu with 投稿を削除.
 * `now` anchors the relative time; a list passes one clock for all its cards.
 */
export function PostCard({
  post,
  detail = false,
  highlighted = false,
  now,
}: {
  post: Post
  detail?: boolean
  highlighted?: boolean
  headline?: 'recipient'
  now?: number
}) {
  const { t, locale } = useText()
  const { me } = useSession()
  const mountedAt = useNow()
  const summary = t('peerbonus.post.summary', { sender: post.sender.displayName, recipient: post.recipient.displayName, points: post.points })
  const sendSame = post.sender.id !== me.member.id && post.recipient.id !== me.member.id
  // `post.clappers` lists the newest claps, so a person can repeat: each member once, newest first.
  const clappers = post.clappers.filter((member, index) => post.clappers.findIndex((other) => other.id === member.id) === index)
  const names = clappers.map((member) => member.displayName).join(t('peerbonus.post.nameSeparator'))
  const clappersText =
    post.clapCount > clappers.length ? t('peerbonus.post.clappedByMore', { names, count: post.clapCount - clappers.length }) : t('peerbonus.post.clappedBy', { names })

  const time = (
    <time dateTime={post.createdAt} title={formatDateTime(post.createdAt, locale)}>
      {detail ? formatDateTime(post.createdAt, locale) : formatRelativeTime(post.createdAt, now ?? mountedAt, locale)}
    </time>
  )
  const stack = <AvatarStack members={clappers} total={Math.max(post.clapCount, clappers.length)} />

  return (
    <article
      aria-label={summary}
      data-highlighted={highlighted || undefined}
      className={cn('rounded-xl border border-line bg-white px-4 py-4 sm:px-5', detail ? 'sm:py-6' : undefined, highlighted ? 'animate-highlight' : undefined)}
    >
      <header className="flex items-start gap-3">
        <Avatar name={post.recipient.displayName} avatarUrl={post.recipient.avatarUrl} size="md" />
        <p className={cn('min-w-0 flex-1 self-center font-semibold break-words text-stone-900', detail ? 'text-base sm:text-xl' : 'text-base')}>
          <Sentence sentence={t('peerbonus.post.to', { name: SLOT })}>
            <HrefLink href={paths.member(post.recipient.id)} className="relative z-[1] rounded hover:underline">
              {post.recipient.displayName}
            </HrefLink>
          </Sentence>
        </p>
        <span
          className={cn(
            'shrink-0 self-center rounded-full bg-accent font-bold whitespace-nowrap text-white tabular-nums shadow-sm',
            detail ? 'px-3 py-1 text-base sm:px-4 sm:py-1.5 sm:text-lg' : 'px-3 py-1 text-base'
          )}
        >
          {t('peerbonus.post.points', { points: post.points })}
        </span>
        <PostMenu post={post} leave={detail} />
      </header>

      <MessageText text={post.message} size={detail ? 'lg' : 'md'} className={detail ? 'mt-4' : 'mt-3'} />

      {post.values.length > 0 ? (
        <ul aria-label={t('peerbonus.post.values')} className="mt-3 flex flex-wrap gap-1.5">
          {post.values.map((key) => (
            <li key={key} className="flex">
              <ValueTag valueKey={key} size={detail ? 'md' : 'sm'} />
            </li>
          ))}
        </ul>
      ) : null}

      <footer className="mt-4 flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <ClapButton post={post} size={detail ? 'md' : 'sm'} />
          {clappers.length > 0 ? (
            detail ? (
              <span className="inline-flex items-center">
                {stack}
                <span className="sr-only">{clappersText}</span>
              </span>
            ) : (
              <HrefLink href={paths.post(post.id)} title={clappersText} className="relative z-[1] inline-flex min-h-10 items-center rounded-full md:min-h-0">
                {stack}
                <span className="sr-only">{clappersText}</span>
              </HrefLink>
            )
          ) : null}
          {sendSame ? (
            <LinkButton href={paths.sendTo(post.recipient.personId)} tone="ghost" size="sm" icon={<Gift />} className="relative z-[1] text-stone-500">
              {t('peerbonus.compose.sendSame')}
              <span className="sr-only">: {post.recipient.displayName}</span>
            </LinkButton>
          ) : null}
        </div>
        <p className="flex min-w-0 flex-wrap items-center gap-x-1.5 text-xs text-stone-500">
          <span className="min-w-0 break-words">
            <Sentence sentence={t('peerbonus.post.from', { name: SLOT })}>
              <HrefLink
                href={paths.member(post.sender.id)}
                className="relative z-[1] inline-flex items-center gap-1 align-middle font-medium text-stone-700 hover:text-stone-900 hover:underline"
              >
                <Avatar name={post.sender.displayName} avatarUrl={post.sender.avatarUrl} size="sm" />
                {post.sender.displayName}
              </HrefLink>
            </Sentence>
          </span>
          <span aria-hidden>·</span>
          {detail ? (
            time
          ) : (
            <HrefLink
              href={paths.post(post.id)}
              aria-label={`${t('peerbonus.post.open')}: ${summary}`}
              className="relative z-[1] inline-flex min-h-10 items-center rounded hover:text-stone-900 hover:underline md:min-h-0"
            >
              {time}
            </HrefLink>
          )}
          {post.slackPermalink ? (
            <>
              <span aria-hidden>·</span>
              <a
                href={post.slackPermalink}
                target="_blank"
                rel="noreferrer"
                className="relative z-[1] inline-flex min-h-10 items-center gap-1 rounded hover:text-stone-900 hover:underline md:min-h-0"
              >
                {t('peerbonus.post.slackOriginal')}
                <ExternalLink aria-hidden className="size-3" />
              </a>
            </>
          ) : null}
        </p>
      </footer>
    </article>
  )
}
