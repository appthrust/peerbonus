import { useSuspenseQuery } from '@tanstack/react-query'
import { CheckCircle2, XCircle } from 'lucide-react'
import { useId, useState, type ReactNode } from 'react'
import type { Statement } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { Badge, cn, formatDateTime, HrefLink, Notice, PageHeader, Section, useDocumentTitle } from '../../ui'
import { CloseStatement } from './close-statement'
import { formatDay, formatMoney, formatMonth } from './format'
import { adminProgramQuery, statementQuery } from './queries'
import { closeState, type CloseCheck } from './statement-state'
import { StatementTable, StatementTotals } from './statement-table'
import { CsvLink } from './statements-page'

/** After a close: confirmation, and the Directory warning when names could not be refreshed. */
function ClosedNotice({ statement }: { statement: Statement }) {
  const { t, locale } = useText()
  return (
    <Notice tone={statement.directoryRefreshed ? 'success' : 'warning'} title={t('peerbonus.admin.statements.closed', { month: formatMonth(statement.month, locale) })}>
      {statement.directoryRefreshed ? null : (
        <>
          <span className="block">{t('peerbonus.admin.statement.directoryStale')}</span>
          <span className="block">{t('peerbonus.admin.statement.directoryStaleBody')}</span>
        </>
      )}
    </Notice>
  )
}

/** One ✓ / ✗ line of the close checklist; an open earlier month links to its own page. */
function CheckLine({ check }: { check: CloseCheck }) {
  const { t, locale } = useText()
  const Icon = check.ok ? CheckCircle2 : XCircle
  let body: ReactNode
  if (check.id === 'monthEnded') {
    body = check.ok ? t('peerbonus.admin.statement.check.ended') : t('peerbonus.admin.statement.check.notEnded', { date: formatDay(check.from, locale) })
  } else if (check.ok || !check.month) {
    body = t('peerbonus.admin.statement.check.previousClosed')
  } else {
    const previous = formatMonth(check.month, locale)
    body = (
      <>
        {t('peerbonus.admin.statement.check.previousOpen', { month: previous })}{' '}
        <HrefLink href={paths.adminStatement(check.month)} className="font-medium whitespace-nowrap text-accent-strong hover:underline">
          {t('peerbonus.admin.statement.check.openPrevious', { month: previous })}
        </HrefLink>
      </>
    )
  }
  return (
    <li className="flex items-start gap-2.5">
      <Icon aria-hidden className={cn('mt-0.5 size-4 shrink-0', check.ok ? 'text-ok' : 'text-stone-400')} />
      <span className="sr-only">{t(check.ok ? 'peerbonus.admin.statement.check.ok' : 'peerbonus.admin.statement.check.missing')}</span>
      <span className={check.ok ? 'text-stone-700' : 'text-stone-900'}>{body}</span>
    </li>
  )
}

/**
 * `/admin/statements/<month>`: the state, then — while open — the close checklist from the
 * server's `closeBlockers` with 締める (enabled only when every line is ✓); once closed, the CSV.
 * Then totals and every line (card rows on phones).
 */
export function StatementPage({ month }: { month: string }) {
  const { t, locale } = useText()
  const checklistId = useId()
  const { data: statement } = useSuspenseQuery(statementQuery(month))
  const { data: program } = useSuspenseQuery(adminProgramQuery)
  const [justClosed, setJustClosed] = useState<Statement | null>(null)
  const monthLabel = formatMonth(month, locale)
  const title = t('peerbonus.admin.statement.title', { month: monthLabel })
  useDocumentTitle(title)
  const state = closeState(statement)

  return (
    <div>
      <PageHeader
        title={title}
        breadcrumbs={[{ label: t('peerbonus.admin.tab.statements'), href: paths.adminStatements }]}
        lead={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <Badge tone={statement.closed ? 'accent' : 'neutral'}>
              {t(statement.closed ? 'peerbonus.admin.statements.status.closed' : 'peerbonus.admin.statements.status.open')}
            </Badge>
            <span>
              {statement.closed && statement.closedAt
                ? statement.closedBy
                  ? t('peerbonus.admin.statement.closedBy', { date: formatDateTime(statement.closedAt, locale), name: statement.closedBy.displayName })
                  : t('peerbonus.admin.statement.closedAt', { date: formatDateTime(statement.closedAt, locale) })
                : t('peerbonus.admin.statement.openLead')}
            </span>
          </span>
        }
        actions={statement.closed ? <CsvLink month={month} /> : undefined}
      />

      <div className="space-y-4">
        {justClosed ? (
          <ClosedNotice statement={justClosed} />
        ) : statement.closed && !statement.directoryRefreshed ? (
          <Notice tone="warning" title={t('peerbonus.admin.statement.directoryStale')}>
            {t('peerbonus.admin.statement.directoryStaleBody')}
          </Notice>
        ) : null}

        {state.kind === 'open' ? (
          <section aria-labelledby={`${checklistId}-title`} className="rounded-lg border border-line bg-white p-4 sm:p-5">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <h2 id={`${checklistId}-title`} className="text-sm font-semibold text-stone-900">
                  {t('peerbonus.admin.statement.checklist')}
                </h2>
                <ul id={checklistId} className="mt-3 space-y-2 text-sm">
                  {state.checks.map((check) => (
                    <CheckLine key={check.id} check={check} />
                  ))}
                </ul>
              </div>
              <div className="shrink-0">
                <CloseStatement
                  month={month}
                  closable={state.closable}
                  describedBy={checklistId}
                  rate={formatMoney(program.pointUnitValue, program.currency, locale)}
                  onClosed={setJustClosed}
                />
              </div>
            </div>
            <p className="mt-4 border-t border-stone-100 pt-3 text-xs text-stone-500">{t('peerbonus.admin.statement.previewNames')}</p>
          </section>
        ) : null}

        <StatementTotals statement={statement} />
      </div>

      <Section title={t('peerbonus.admin.statement.lines')} lead={t('peerbonus.admin.statements.csvNote')} className="mt-8">
        {statement.lines.length === 0 ? (
          <p className="rounded-lg border border-dashed border-stone-300 bg-white px-4 py-6 text-center text-sm text-stone-500">{t('peerbonus.admin.statement.empty')}</p>
        ) : (
          <StatementTable statement={statement} caption={title} />
        )}
      </Section>
    </div>
  )
}
