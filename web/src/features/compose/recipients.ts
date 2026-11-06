// Pure parts of the recipient picker: the instant filter, merging a server
// search into the loaded people, the sections with each person once, resolving
// `?to=`, and roving-focus moves through whatever the sections laid out.
import type { Candidate, Member, Recipients, RecipientGroup } from '../../api/peerbonus'

/** Case-, width- and space-insensitive form for matching ("佐藤花子" finds "佐藤 花子"). */
const fold = (text: string) => text.normalize('NFKC').toLowerCase().replace(/\s+/g, '')

/** The people whose name or email contains `query`, in their order; an empty query keeps everyone. */
export function filterCandidates(items: readonly Candidate[], query: string): Candidate[] {
  const needle = fold(query)
  if (!needle) return [...items]
  return items.filter((item) => fold(item.displayName).includes(needle) || fold(item.email).includes(needle))
}

/** `local` first, then the people only the server search found, each person once. */
export function mergeCandidates(local: readonly Candidate[], remote: readonly Candidate[]): Candidate[] {
  const seen = new Set(local.map((item) => item.personId))
  return [...local, ...remote.filter((item) => !seen.has(item.personId))]
}

/** The candidate as picked from `group`'s tab: that group leads the ones the server knew. */
export function pickedFrom(candidate: Candidate, group: RecipientGroup): Candidate {
  return { ...candidate, groups: [group, ...candidate.groups.filter((known) => known.id !== group.id)] }
}

/**
 * The person `?to=<personId>` names: from the picker's people, else a Member some loaded page
 * showed (their groups unknown). Undefined for the caller and for anyone not found.
 */
export function findRecipient(personId: string, recipients: Recipients | undefined, member: Member | undefined, selfPersonId: string): Candidate | undefined {
  if (personId === selfPersonId) return undefined
  const known = [...(recipients?.recent ?? []), ...(recipients?.teams.flatMap((team) => team.members) ?? []), ...(recipients?.everyone.items ?? [])]
  const found = known.find((item) => item.personId === personId)
  if (found) return found
  return member ? { personId: member.personId, displayName: member.displayName, email: member.email, pending: member.pending, groups: [], avatarUrl: member.avatarUrl } : undefined
}

/** Who the picker shows, each person once (最近 > チーム > 全員), narrowed by `query`. */
export type PickerPeople = {
  recent: Candidate[]
  /** Teams in the server's order (deepest first) with their remaining people; empty teams left out. */
  teams: { group: RecipientGroup; members: Candidate[] }[]
  /** Everyone not shown above, name order, then people only the server search found. */
  everyone: Candidate[]
}

/**
 * The picker's sections for `recipients` and `query`: the recent people, the caller's teams and
 * everyone, with a person who already appeared in an earlier section left out of the later ones.
 * `remote` (a server search's answer) joins everyone. A team's people carry that team first.
 */
export function pickerPeople(recipients: Recipients | undefined, query: string, remote: readonly Candidate[] = []): PickerPeople {
  const seen = new Set<string>()
  const fresh = (items: readonly Candidate[]) => {
    const kept = filterCandidates(items, query).filter((item) => !seen.has(item.personId))
    for (const item of kept) seen.add(item.personId)
    return kept
  }
  const recent = fresh(recipients?.recent ?? [])
  const teams = (recipients?.directoryAvailable ? recipients.teams : [])
    .map((team) => ({ group: team.group, members: fresh(team.members).map((item) => pickedFrom(item, team.group)) }))
    .filter((team) => team.members.length > 0)
  const everyone = fresh(mergeCandidates(recipients?.directoryAvailable ? recipients.everyone.items : [], filterCandidates(remote, query)))
  return { recent, teams, everyone }
}

/** A person button's top-left corner, as laid out on screen. */
export type Spot = { x: number; y: number }

/** Spots this close vertically share a row (chips and cards differ a little in height). */
const ROW_TOLERANCE = 6

/**
 * Where a roving-focus key moves among people laid out at `spots` (in reading order, across every
 * section): left/right step through the order, Home/End jump, up/down go to the nearest row above
 * or below and the person closest in that column, staying put at an edge. Undefined for other keys.
 */
export function moveSpatial(key: string, index: number, spots: readonly Spot[]): number | undefined {
  const last = spots.length - 1
  switch (key) {
    case 'ArrowRight':
      return Math.min(index + 1, last)
    case 'ArrowLeft':
      return Math.max(index - 1, 0)
    case 'Home':
      return 0
    case 'End':
      return last
    case 'ArrowDown':
    case 'ArrowUp': {
      const here = spots[index]
      if (!here) return index
      const down = key === 'ArrowDown'
      const beyond = spots.map((spot, position) => ({ spot, position })).filter(({ spot }) => (down ? spot.y > here.y + ROW_TOLERANCE : spot.y < here.y - ROW_TOLERANCE))
      if (beyond.length === 0) return index
      const rowY = down ? Math.min(...beyond.map(({ spot }) => spot.y)) : Math.max(...beyond.map(({ spot }) => spot.y))
      const row = beyond.filter(({ spot }) => Math.abs(spot.y - rowY) <= ROW_TOLERANCE)
      return row.reduce((best, item) => (Math.abs(item.spot.x - here.x) < Math.abs(best.spot.x - here.x) ? item : best)).position
    }
    default:
      return undefined
  }
}
