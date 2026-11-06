import type { Member } from '../../api/peerbonus'
import { paths } from '../../routes/paths'
import { Avatar, cn, HrefLink } from '../../ui'

/** Avatar and name, linking to the member page. */
export function MemberLink({ member, size = 'sm', className }: { member: Member; size?: 'sm' | 'md'; className?: string }) {
  return (
    <HrefLink
      href={paths.member(member.id)}
      className={cn('relative z-[1] inline-flex min-h-10 max-w-full min-w-0 items-center gap-2 rounded-md font-medium text-stone-800 hover:text-stone-950 hover:underline md:min-h-0', className)}
    >
      <Avatar name={member.displayName} avatarUrl={member.avatarUrl} size={size} />
      <span className="truncate">{member.displayName}</span>
    </HrefLink>
  )
}
