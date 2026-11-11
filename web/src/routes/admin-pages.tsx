// Admin routes (`/admin/*`): a layout with the section switch and six
// sections (参加状況, インサイト, つながり, 月次明細, 設定, Slack). The layout resolves the caller's admin role from `GET /me`
// (already loaded by the root); for a participant it renders the admins-only
// page and the section loaders skip their admin-only reads (which would be 403).
import { createRoute, notFound, redirect, useNavigate } from '@tanstack/react-router'
import { isPeerbonusError } from '../api/peerbonus'
import { configQuery, insightsQuery, meQuery, networkQuery } from '../api/queries'
import { AdminLayout } from '../features/admin/admin-layout'
import { InsightsPage } from '../features/admin/insights-page'
import { NetworkPage, networkSearch, type NetworkSearch } from '../features/admin/network/network-page'
import { ParticipationPage } from '../features/admin/participation-page'
import { ProgramPage } from '../features/admin/program-page'
import { adminMembersQuery, adminProgramQuery, slackDeliveriesQuery, slackQuery, statementQuery, statementsQuery } from '../features/admin/queries'
import { SlackPage } from '../features/admin/slack/slack-page'
import { StatementPage } from '../features/admin/statement-page'
import { StatementsPage } from '../features/admin/statements-page'
import { isMonthKey, monthKeyAt } from '../model/period'
import { notFoundOn404, rootRoute } from './root'

export const adminRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/admin',
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQuery)
    return { isAdmin: me.roles.peerbonusAdmin, timeZone: me.program.timeZone }
  },
  component: function AdminRoute() {
    const { isAdmin } = adminRoute.useRouteContext()
    return <AdminLayout isAdmin={isAdmin} />
  },
})

/** `/admin` opens the first section, 参加状況. */
const adminIndexRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/admin/members', replace: true })
  },
})

type MonthSearch = { month?: string }

/** `?month=YYYY-MM` of 参加状況 and インサイト; anything else is this month. */
const monthSearch = (search: Record<string, unknown>): MonthSearch => (typeof search.month === 'string' && isMonthKey(search.month) ? { month: search.month } : {})

export const adminMembersRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: 'members',
  validateSearch: monthSearch,
  loaderDeps: ({ search }) => ({ month: search.month }),
  loader: async ({ context, deps }) => {
    if (!context.isAdmin) return
    await Promise.all([
      context.queryClient.ensureQueryData(adminMembersQuery(deps.month ?? monthKeyAt(Date.now(), context.timeZone))),
      context.queryClient.ensureQueryData(statementsQuery),
    ])
  },
  component: function AdminMembersRoute() {
    const { month } = adminMembersRoute.useSearch()
    const { timeZone } = adminMembersRoute.useRouteContext()
    const navigate = useNavigate({ from: adminMembersRoute.fullPath })
    const currentMonth = monthKeyAt(Date.now(), timeZone)
    return <ParticipationPage month={month ?? currentMonth} currentMonth={currentMonth} onMonthChange={(next) => void navigate({ search: next === currentMonth ? {} : { month: next } })} />
  },
})

export const adminInsightsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: 'insights',
  validateSearch: monthSearch,
  loaderDeps: ({ search }) => ({ month: search.month }),
  loader: async ({ context, deps }) => {
    if (!context.isAdmin) return
    await Promise.all([
      context.queryClient.ensureQueryData(insightsQuery(deps.month ?? monthKeyAt(Date.now(), context.timeZone))),
      context.queryClient.ensureQueryData(statementsQuery),
    ])
  },
  component: function AdminInsightsRoute() {
    const { month } = adminInsightsRoute.useSearch()
    const { timeZone } = adminInsightsRoute.useRouteContext()
    const navigate = useNavigate({ from: adminInsightsRoute.fullPath })
    const currentMonth = monthKeyAt(Date.now(), timeZone)
    return <InsightsPage month={month ?? currentMonth} currentMonth={currentMonth} onMonthChange={(next) => void navigate({ search: next === currentMonth ? {} : { month: next } })} />
  },
})

/** `?month=YYYY-MM&group=<Directory group id>` of つながり; a missing or malformed month is this month, no group is everyone. */
const networkSearchParams = (search: Record<string, unknown>): NetworkSearch => ({
  ...monthSearch(search),
  ...(typeof search.group === 'string' && search.group !== '' ? { group: search.group } : {}),
})

export const adminNetworkRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: 'network',
  validateSearch: networkSearchParams,
  loaderDeps: ({ search }) => ({ month: search.month, group: search.group }),
  loader: async ({ context, deps }) => {
    if (!context.isAdmin) return
    await Promise.all([
      context.queryClient.ensureQueryData(networkQuery(deps.month ?? monthKeyAt(Date.now(), context.timeZone), deps.group)).catch((error: unknown) => {
        // A group Directory no longer lists (an old link): show everyone instead of an error.
        if (isPeerbonusError(error, 'group_not_found')) throw redirect({ to: '/admin/network', search: deps.month ? { month: deps.month } : {}, replace: true })
        throw error
      }),
      context.queryClient.ensureQueryData(statementsQuery),
    ])
  },
  component: function AdminNetworkRoute() {
    const { month, group } = adminNetworkRoute.useSearch()
    const { timeZone } = adminNetworkRoute.useRouteContext()
    const navigate = useNavigate({ from: adminNetworkRoute.fullPath })
    const currentMonth = monthKeyAt(Date.now(), timeZone)
    return (
      <NetworkPage
        month={month ?? currentMonth}
        group={group ?? ''}
        currentMonth={currentMonth}
        onChange={(next) => void navigate({ search: networkSearch(next.month, next.group, currentMonth) })}
      />
    )
  },
})

export const adminProgramRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: 'program',
  loader: async ({ context }) => {
    if (context.isAdmin) await context.queryClient.ensureQueryData(adminProgramQuery)
  },
  component: ProgramPage,
})

export const adminSlackRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: 'slack',
  loader: async ({ context }) => {
    if (!context.isAdmin) return
    await Promise.all([
      context.queryClient.ensureQueryData(slackQuery),
      context.queryClient.ensureQueryData(configQuery),
      context.queryClient.ensureQueryData(slackDeliveriesQuery(null)),
      // The conversation card checks points and values against the Program.
      context.queryClient.ensureQueryData(adminProgramQuery),
    ])
  },
  component: SlackPage,
})

export const adminStatementsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: 'statements',
  loader: async ({ context }) => {
    if (!context.isAdmin) return
    const program = await context.queryClient.ensureQueryData(adminProgramQuery)
    await Promise.all([
      context.queryClient.ensureQueryData(statementsQuery),
      context.queryClient.ensureQueryData(statementQuery(monthKeyAt(Date.now(), program.timeZone))),
    ])
  },
  component: StatementsPage,
})

export const adminStatementRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: 'statements/$month',
  loader: async ({ context, params }) => {
    // A malformed month is a wrong address, not a server question.
    if (!isMonthKey(params.month)) throw notFound()
    if (!context.isAdmin) return
    await Promise.all([context.queryClient.ensureQueryData(statementQuery(params.month)).catch(notFoundOn404), context.queryClient.ensureQueryData(adminProgramQuery)])
  },
  component: function AdminStatementRoute() {
    const { month } = adminStatementRoute.useParams()
    return <StatementPage key={month} month={month} />
  },
})

/** Spread into the root's children. */
export const adminRoutes = [
  adminRoute.addChildren([adminIndexRoute, adminMembersRoute, adminInsightsRoute, adminNetworkRoute, adminStatementsRoute, adminStatementRoute, adminProgramRoute, adminSlackRoute]),
] as const
