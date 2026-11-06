import { Outlet, useRouterState } from '@tanstack/react-router'
import { ShieldAlert } from 'lucide-react'
import type { MessageId } from '../../copy'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { EmptyState, LinkButton, Tabs, useDocumentTitle } from '../../ui'

/** The sections, dashboard first (principle 10). `monthly`: the section reads `?month=`, which the switch carries over. */
const TABS: readonly { href: string; label: MessageId; monthly: boolean }[] = [
  { href: paths.adminMembers, label: 'peerbonus.admin.tab.members', monthly: true },
  { href: paths.adminInsights, label: 'peerbonus.admin.tab.insights', monthly: true },
  { href: paths.adminNetwork, label: 'peerbonus.admin.tab.network', monthly: true },
  { href: paths.adminStatements, label: 'peerbonus.admin.tab.statements', monthly: false },
  { href: paths.adminProgram, label: 'peerbonus.admin.tab.program', monthly: false },
  { href: paths.adminSlack, label: 'peerbonus.admin.tab.slack', monthly: false },
]

/** Shown instead of the admin pages to a participant who opened an `/admin` URL. */
function AdminOnly() {
  const { t } = useText()
  useDocumentTitle(t('peerbonus.admin.forbiddenTitle'))
  return (
    <div className="mx-auto max-w-md py-12">
      <EmptyState
        icon={<ShieldAlert />}
        heading="h1"
        title={t('peerbonus.admin.forbiddenTitle')}
        body={t('peerbonus.admin.forbiddenBody')}
        action={<LinkButton href={paths.home}>{t('peerbonus.notFound.home')}</LinkButton>}
      />
    </div>
  )
}

/**
 * `/admin/*`: the section switch (参加状況 / インサイト / つながり / 月次明細 / 設定 / Slack) above the current admin
 * page. The month picked on 参加状況, インサイト or つながり (`?month=`) stays when switching between them;
 * a statement's detail page keeps 月次明細 current.
 */
export function AdminLayout({ isAdmin }: { isAdmin: boolean }) {
  const { t } = useText()
  // `?month=` of the current URL, whichever admin route parsed it.
  const month = useRouterState({
    select: (state) => {
      const search: object = state.location.search
      return 'month' in search && typeof search.month === 'string' ? search.month : ''
    },
  })
  if (!isAdmin) return <AdminOnly />
  const monthSearch = month ? `?month=${encodeURIComponent(month)}` : ''
  return (
    <div className="mx-auto max-w-5xl">
      <Tabs
        label={t('peerbonus.admin.tabsLabel')}
        className="mb-6"
        items={TABS.map((tab) => ({ href: tab.monthly ? `${tab.href}${monthSearch}` : tab.href, label: t(tab.label), exact: false }))}
      />
      <Outlet />
    </div>
  )
}
