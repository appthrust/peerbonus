// Admin read models. Keys live under PEERBONUS_KEY, so the participant
// screens' invalidation after a write (post, clap, delete) refreshes them too.
import { queryOptions } from '@tanstack/react-query'
import { peerbonusApi, type SlackDeliveryState } from '../../api/peerbonus'
import { PEERBONUS_KEY } from '../../api/queries'

const ADMIN_KEY = [...PEERBONUS_KEY, 'admin'] as const

export const adminProgramQuery = queryOptions({ queryKey: [...ADMIN_KEY, 'program'], queryFn: () => peerbonusApi.admin.program() })

export const statementsQuery = queryOptions({ queryKey: [...ADMIN_KEY, 'statements'], queryFn: () => peerbonusApi.admin.statements() })

export const statementQuery = (month: string) =>
  queryOptions({ queryKey: [...ADMIN_KEY, 'statements', month], queryFn: () => peerbonusApi.admin.statement(month) })

export const adminMembersQuery = (month: string) =>
  queryOptions({ queryKey: [...ADMIN_KEY, 'members', month], queryFn: () => peerbonusApi.admin.members(month) })

/** Every Slack read (status, channels, deliveries), for invalidation after a write. */
export const SLACK_KEY = [...ADMIN_KEY, 'slack'] as const

/** The Slack connection, settings and delivery counts. Writes replace it with their answer, then invalidate SLACK_KEY. */
export const slackQuery = queryOptions({ queryKey: SLACK_KEY, queryFn: () => peerbonusApi.admin.slack.get() })

/** Channel candidates for a typed prefix; the server caches Slack's list for a minute. */
export const slackChannelsQuery = (q: string) =>
  queryOptions({ queryKey: [...SLACK_KEY, 'channels', q], queryFn: () => peerbonusApi.admin.slack.channels(q), staleTime: 60_000 })

/** Recent deliveries, all states or one; refreshed every 15 s while the page is open (the worker moves them). */
export const slackDeliveriesQuery = (state: SlackDeliveryState | null) =>
  queryOptions({
    queryKey: [...SLACK_KEY, 'deliveries', state ?? 'all'],
    queryFn: () => peerbonusApi.admin.slack.deliveries(state ?? undefined),
    refetchInterval: 15_000,
  })
