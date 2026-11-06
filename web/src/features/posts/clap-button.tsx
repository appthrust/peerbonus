import { useMutation, useQueryClient } from '@tanstack/react-query'
import { PartyPopper } from 'lucide-react'
import { useId, useRef, useState } from 'react'
import { isPeerbonusError, newIdempotencyKey, peerbonusApi, type Member, type Post } from '../../api/peerbonus'
import { invalidatePeerbonus, storePost } from '../../api/queries'
import { useText } from '../../i18n'
import { useSession } from '../../shell/use-session'
import { ActionError, cn, Tooltip } from '../../ui'

/**
 * Whether the caller can clap now: not on a post they sent or received, not on a post of a closed
 * month (known once the server said so), and only with the cost left this week.
 */
export type ClapAvailability = 'ready' | 'own' | 'closed' | 'noBalance'

export function clapAvailability(
  post: { sender: { id: string }; recipient: { id: string } },
  viewer: { memberId: string; remaining: number; cost: number; closed?: boolean }
): ClapAvailability {
  if (post.sender.id === viewer.memberId || post.recipient.id === viewer.memberId) return 'own'
  if (viewer.closed) return 'closed'
  return viewer.remaining < viewer.cost ? 'noBalance' : 'ready'
}

const REASON = { own: 'peerbonus.clap.own', closed: 'peerbonus.clap.periodClosed', noBalance: 'peerbonus.clap.noBalance' } as const

/** The post as it looks once the caller's clap lands: one more clap, theirs, and them first among the clappers. */
function withMyClap(post: Post, me: Member): Post {
  return {
    ...post,
    clapCount: post.clapCount + 1,
    clappedByMe: post.clappedByMe + 1,
    clappers: [me, ...post.clappers.filter((member) => member.id !== me.id)].slice(0, 3),
  }
}

/**
 * 拍手: every press is a new clap (2 pt from the caller, +1 to the sender and
 * the recipient); claps are never taken back. The press pops the icon and the
 * count moves at once (optimistic); a refusal puts the post back. One press
 * keeps its `Idempotency-Key` until the server answers, so pressing again
 * after an unanswered attempt (network, rollout) cannot clap twice. The count
 * shows everyone's claps; "あなた ×n" the caller's own. A disabled button says
 * why in a tooltip (and to screen readers).
 */
export function ClapButton({ post, size = 'sm' }: { post: Post; size?: 'sm' | 'md' }) {
  const { t } = useText()
  const { me } = useSession()
  const client = useQueryClient()
  const describedBy = useId()
  const attemptKey = useRef<string | null>(null)
  // Bumped per press so the icon's pop animation replays (a new key remounts it).
  const [presses, setPresses] = useState(0)
  const [closed, setClosed] = useState(false)
  const cost = me.program.clapCost
  const availability = clapAvailability(post, { memberId: me.member.id, remaining: me.stats.period.remaining, cost, closed })
  const mutation = useMutation({
    mutationFn: () => {
      attemptKey.current ??= newIdempotencyKey()
      return peerbonusApi.posts.clap(post.id, attemptKey.current)
    },
    onMutate: () => {
      storePost(client, withMyClap(post, me.member))
      return { before: post }
    },
    onSuccess: async (result) => {
      attemptKey.current = null
      storePost(client, result.post)
      // The balance, stats and the clap list all moved.
      await invalidatePeerbonus(client)
    },
    onError: async (error, _variables, context) => {
      if (context) storePost(client, context.before)
      if (isPeerbonusError(error, 'period_closed')) setClosed(true)
      // Only an unanswered attempt may have landed; anything the server answered is final.
      if (!(isPeerbonusError(error) && error.transient)) attemptKey.current = null
      await invalidatePeerbonus(client)
    },
  })
  const reason = availability === 'ready' ? undefined : t(REASON[availability])
  const mine = post.clappedByMe > 0
  const button = (
    <button
      type="button"
      onClick={() => {
        setPresses((count) => count + 1)
        mutation.mutate()
      }}
      disabled={availability !== 'ready' || mutation.isPending}
      aria-describedby={describedBy}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border font-medium transition-[transform,background-color,border-color,color] duration-150 select-none active:scale-90 disabled:cursor-not-allowed disabled:active:scale-100',
        size === 'md' ? 'h-10 px-4 text-sm' : 'h-10 px-3 text-xs md:h-8',
        mine ? 'border-accent-line bg-accent-soft text-accent-strong' : 'border-stone-300 bg-white text-stone-700',
        availability === 'ready' ? 'hover:border-accent hover:text-accent-strong' : 'opacity-60'
      )}
    >
      <PartyPopper key={presses} aria-hidden className={cn('size-4', presses > 0 ? 'animate-clap' : undefined)} />
      {t('peerbonus.clap.label')}
      <span className="tabular-nums" aria-live="polite">
        {post.clapCount}
      </span>
    </button>
  )
  return (
    <div className="relative z-[1] flex flex-col items-start gap-2">
      <div className="flex flex-wrap items-center gap-2">
        {/* One wrapper in every state, so a press that empties the balance keeps focus. A disabled
            button gets no hover or focus, so then the wrapper takes focus and carries the hint. */}
        <Tooltip content={reason ?? t('peerbonus.clap.action', { cost })}>
          <span tabIndex={reason ? 0 : undefined} className="inline-flex rounded-full">
            {button}
          </span>
        </Tooltip>
        {mine ? <span className="text-xs font-medium text-accent-strong">{t('peerbonus.clap.mine', { count: post.clappedByMe })}</span> : null}
        <span id={describedBy} className="sr-only">
          {reason ?? t('peerbonus.clap.action', { cost })}
        </span>
      </div>
      {mutation.isError ? <ActionError error={mutation.error} /> : null}
    </div>
  )
}
