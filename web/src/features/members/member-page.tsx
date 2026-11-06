import { useSuspenseQuery } from '@tanstack/react-query'
import { ExternalLink, Gift, Inbox, Send } from 'lucide-react'
import { memberQuery } from '../../api/queries'
import { useText } from '../../i18n'
import { timeUntil } from '../../model/period'
import { paths, type MemberTab } from '../../routes/paths'
import { useSession } from '../../shell/use-session'
import { Avatar, Badge, Chip, CLOCK_TICK_MS, EmptyState, LinkButton, Ring, Section, StatCard, Tabs, useDocumentTitle, useNow } from '../../ui'
import { formatCount } from '../admin/format'
import { PostList } from '../timeline/post-list'
import { MonthChart, PartnerList, PendingMark, ValueBars } from './profile-parts'

/** A small empty note inside a profile block, with the next action in it (principle 8). */
function EmptyNote({ text, action }: { text: string; action?: { href: string; label: string } }) {
  return (
    <div className="flex flex-col items-start gap-2 rounded-lg border border-dashed border-stone-300 bg-white px-4 py-4 text-sm text-stone-500 sm:flex-row sm:items-center sm:justify-between">
      <p>{text}</p>
      {action ? (
        <LinkButton size="sm" href={action.href}>
          {action.label}
        </LinkButton>
      ) : null}
    </div>
  )
}

/**
 * `/members/<id>` (and `/me`, which forwards here), the profile story: who they are (Directory
 * groups and profile, 贈る), this week's balance on one's own page, this month and in total,
 * the last six months, top partners, the values they received, then their received / sent posts
 * (tabs, `?tab=sent`).
 */
export function MemberPage({ memberId, tab }: { memberId: string; tab: MemberTab }) {
  const { t, locale } = useText()
  const { me } = useSession()
  const { data: profile } = useSuspenseQuery(memberQuery(memberId))
  const { member, groups, stats } = profile
  const isMe = member.id === me.member.id
  useDocumentTitle(member.displayName)
  const week = me.stats.period
  const now = useNow(CLOCK_TICK_MS)
  const resets = timeUntil(week.endsAt, now)
  const values = me.program.values
  const name = member.displayName
  const sendToThem = { href: paths.sendTo(member.personId), label: t('peerbonus.member.sendLabel', { name }) }
  const sendSomeone = { href: paths.home, label: t('peerbonus.member.sendFirst') }
  const timeline = { href: paths.home, label: t('peerbonus.member.openTimeline') }

  return (
    <div className="mx-auto max-w-3xl">
      <header className="mb-8 flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-center gap-4">
          <Avatar name={name} avatarUrl={member.avatarUrl} size="lg" />
          <div className="min-w-0">
            <h1 className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xl font-semibold tracking-tight break-words text-stone-900 sm:text-2xl">
              <span className="min-w-0 break-words">{name}</span>
              {member.pending ? <PendingMark /> : null}
              {isMe ? <Badge tone="accent">{t('peerbonus.nav.you')}</Badge> : null}
            </h1>
            {groups.length > 0 ? (
              <ul aria-label={t('peerbonus.member.groups')} className="mt-2 flex flex-wrap gap-1.5">
                {groups.map((group) => (
                  <li key={group.id} className="min-w-0">
                    <Chip title={group.path}>{group.name}</Chip>
                  </li>
                ))}
              </ul>
            ) : null}
            {member.directoryUrl ? (
              <a href={member.directoryUrl} className="mt-1.5 inline-flex min-h-10 items-center gap-1 text-sm text-accent-strong hover:underline md:min-h-0">
                {t('peerbonus.member.directoryProfile')}
                <ExternalLink aria-hidden className="size-3.5" />
              </a>
            ) : null}
          </div>
        </div>
        {isMe ? (
          <div className="flex items-center gap-4 rounded-xl border border-line bg-white px-4 py-3">
            <Ring value={week.remaining} max={week.allowance} size={72} label={t('peerbonus.member.weekRing', { remaining: week.remaining, allowance: week.allowance })}>
              <span className="text-lg font-semibold text-stone-900 tabular-nums">{formatCount(week.remaining, locale)}</span>
              <span className="text-[10px] font-medium text-stone-500">{t('peerbonus.common.pointsUnit')}</span>
            </Ring>
            <div className="min-w-0 text-sm">
              <p className="font-medium text-stone-900">{t('peerbonus.nav.balance', { remaining: formatCount(week.remaining, locale) })}</p>
              <p className="text-stone-500">{t('peerbonus.member.weekAllowance', { allowance: formatCount(week.allowance, locale) })}</p>
              <p className="text-xs text-stone-500">
                {resets.days > 0 ? t('peerbonus.common.resetsInDays', { days: resets.days }) : t('peerbonus.common.resetsInHours', { hours: resets.hours })}
              </p>
            </div>
          </div>
        ) : (
          <LinkButton href={sendToThem.href} tone="primary" icon={<Gift />} aria-label={sendToThem.label} className="self-start">
            {t('peerbonus.member.send')}
          </LinkButton>
        )}
      </header>

      <dl aria-label={t('peerbonus.member.stats')} className="grid grid-cols-2 gap-2 sm:grid-cols-4 sm:gap-3">
        <StatCard label={t('peerbonus.member.monthReceived')} value={formatCount(stats.month.receivedPoints, locale)} unit={t('peerbonus.common.pointsUnit')} />
        <StatCard label={t('peerbonus.member.totalReceived')} value={formatCount(stats.total.receivedPoints, locale)} unit={t('peerbonus.common.pointsUnit')} />
        <StatCard label={t('peerbonus.member.receivedPosts')} value={formatCount(stats.total.receivedPosts, locale)} unit={t('peerbonus.member.postsUnit')} />
        <StatCard label={t('peerbonus.member.sentPosts')} value={formatCount(stats.total.sentPosts, locale)} unit={t('peerbonus.member.postsUnit')} />
      </dl>

      <Section title={t('peerbonus.member.months')} lead={t('peerbonus.member.monthsLead')}>
        <MonthChart months={stats.months} />
      </Section>

      <div className="mt-8 grid gap-8 md:grid-cols-2 md:gap-6">
        {/* One wrapper per column keeps each Section its parent's first child (no top margin). */}
        <div className="min-w-0">
          <Section title={t('peerbonus.member.sentTo')} lead={t('peerbonus.member.partnersLead')}>
            <PartnerList
              label={t('peerbonus.member.sentTo')}
              partners={stats.partners.sentTo}
              empty={<EmptyNote text={isMe ? t('peerbonus.member.sentToEmptyMe') : t('peerbonus.member.sentToEmpty', { name })} action={isMe ? sendSomeone : timeline} />}
            />
          </Section>
        </div>
        <div className="min-w-0">
          <Section title={t('peerbonus.member.receivedFrom')} lead={t('peerbonus.member.partnersLead')}>
            <PartnerList
              label={t('peerbonus.member.receivedFrom')}
              partners={stats.partners.receivedFrom}
              empty={<EmptyNote text={isMe ? t('peerbonus.member.receivedFromEmptyMe') : t('peerbonus.member.receivedFromEmpty', { name })} action={isMe ? timeline : sendToThem} />}
            />
          </Section>
        </div>
      </div>

      <Section title={t('peerbonus.member.values')} lead={t('peerbonus.member.valuesLead')}>
        <ValueBars
          label={t('peerbonus.member.values')}
          items={stats.values}
          values={values}
          empty={<EmptyNote text={t('peerbonus.member.valuesEmpty')} action={isMe ? timeline : sendToThem} />}
        />
      </Section>

      <section className="mt-10">
        <Tabs
          label={t('peerbonus.member.tabs')}
          className="mb-4"
          items={[
            { href: paths.member(member.id, 'received'), label: t('peerbonus.member.tabReceived') },
            { href: paths.member(member.id, 'sent'), label: t('peerbonus.member.tabSent') },
          ]}
        />
        {/* Each tab keeps its own pages; switching back shows what was already loaded. */}
        <PostList
          key={tab}
          feed={{ kind: tab, memberId: member.id }}
          now={now}
          empty={
            tab === 'sent' ? (
              <EmptyState
                icon={<Send />}
                title={isMe ? t('peerbonus.member.emptySentMe') : t('peerbonus.member.emptySent', { name })}
                action={<LinkButton href={isMe ? sendSomeone.href : timeline.href}>{isMe ? sendSomeone.label : timeline.label}</LinkButton>}
              />
            ) : (
              <EmptyState
                icon={<Inbox />}
                title={isMe ? t('peerbonus.member.emptyReceivedMe') : t('peerbonus.member.emptyReceived', { name })}
                action={
                  isMe ? (
                    <LinkButton href={timeline.href}>{timeline.label}</LinkButton>
                  ) : (
                    <LinkButton tone="primary" icon={<Gift />} href={sendToThem.href}>
                      {sendToThem.label}
                    </LinkButton>
                  )
                }
              />
            )
          }
        />
      </section>
    </div>
  )
}
