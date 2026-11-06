import { useSuspenseQuery } from '@tanstack/react-query'
import { clapsQuery, postQuery } from '../../api/queries'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { useSession } from '../../shell/use-session'
import { Avatar, Badge, CLOCK_TICK_MS, formatDateTime, formatRelativeTime, HrefLink, PageHeader, Section, useDocumentTitle, useNow } from '../../ui'
import { PostCard } from './post-card'

/**
 * `/posts/<id>`: the post as a large card (clap from here too; admins delete
 * through its menu) and everyone who clapped, newest first.
 */
export function PostPage({ postId }: { postId: string }) {
  const { t, locale } = useText()
  const { me } = useSession()
  const { data: post } = useSuspenseQuery(postQuery(postId))
  const { data: claps } = useSuspenseQuery(clapsQuery(postId))
  useDocumentTitle(t('peerbonus.post.summary', { sender: post.sender.displayName, recipient: post.recipient.displayName, points: post.points }))
  const now = useNow(CLOCK_TICK_MS)
  return (
    <div className="mx-auto max-w-2xl">
      <PageHeader title={t('peerbonus.post.title')} breadcrumbs={[{ label: t('peerbonus.nav.home'), href: paths.home }]} />
      <PostCard post={post} detail />
      <Section title={t('peerbonus.post.claps')} lead={claps.length > 0 ? t('peerbonus.post.clapsCount', { count: post.clapCount }) : undefined} className="mt-8">
        {claps.length === 0 ? (
          <p className="rounded-lg border border-dashed border-stone-300 bg-white px-4 py-6 text-center text-sm text-stone-500">{t('peerbonus.post.clapsEmpty')}</p>
        ) : (
          <ul className="divide-y divide-line rounded-xl border border-line bg-white">
            {claps.map((clap) => (
              <li key={clap.id} className="flex items-center gap-3 px-4 py-2.5">
                <Avatar name={clap.member.displayName} avatarUrl={clap.member.avatarUrl} size="md" />
                <span className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-0.5">
                  <HrefLink href={paths.member(clap.member.id)} className="min-w-0 rounded font-medium break-words text-stone-800 hover:text-stone-950 hover:underline">
                    {clap.member.displayName}
                  </HrefLink>
                  {clap.member.id === me.member.id ? <Badge tone="accent">{t('peerbonus.nav.you')}</Badge> : null}
                </span>
                <time dateTime={clap.createdAt} title={formatDateTime(clap.createdAt, locale)} className="shrink-0 text-xs text-stone-500">
                  {formatRelativeTime(clap.createdAt, now, locale)}
                </time>
              </li>
            ))}
          </ul>
        )}
      </Section>
    </div>
  )
}
