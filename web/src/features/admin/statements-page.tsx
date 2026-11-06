import { useSuspenseQuery } from '@tanstack/react-query'
import { CalendarRange, ChevronRight, Download } from 'lucide-react'
import { statementCsvUrl, type StatementSummary } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { monthKeyAt } from '../../model/period'
import { paths } from '../../routes/paths'
import { Badge, buttonClasses, CLOCK_TICK_MS, EmptyState, formatDateTime, HrefLink, LinkButton, PageHeader, Section, useDocumentTitle, useNow } from '../../ui'
import { formatCount, formatDecimal, formatMoney, formatMonth } from './format'
import { adminProgramQuery, statementQuery, statementsQuery } from './queries'
import { StatementTotals } from './statement-table'

/** Download link for a closed month's CSV (the server sends it as an attachment). */
export function CsvLink({ month, compact = false }: { month: string; compact?: boolean }) {
  const { t, locale } = useText()
  return (
    <a
      href={statementCsvUrl(month)}
      download={`peerbonus-${month}.csv`}
      aria-label={compact ? t('peerbonus.admin.statements.csvMonth', { month: formatMonth(month, locale) }) : undefined}
      className={buttonClasses('secondary', compact ? 'sm' : 'md')}
    >
      <Download aria-hidden />
      {t(compact ? 'peerbonus.admin.statements.csv' : 'peerbonus.admin.statements.csvDownload')}
    </a>
  )
}

/** 締め済み / 今月 / 未締め, with the close time under a closed month. */
function MonthState({ item, currentMonth }: { item: StatementSummary; currentMonth: string }) {
  const { t, locale } = useText()
  return (
    <div className="flex flex-col items-start gap-1">
      {item.closed ? (
        <Badge tone="accent">{t('peerbonus.admin.statements.status.closed')}</Badge>
      ) : (
        <Badge>{t(item.month === currentMonth ? 'peerbonus.admin.statements.status.current' : 'peerbonus.admin.statements.status.open')}</Badge>
      )}
      {item.closedAt ? <span className="text-xs text-stone-500">{t('peerbonus.admin.statement.closedAt', { date: formatDateTime(item.closedAt, locale) })}</span> : null}
    </div>
  )
}

/** Every month, newest first: a table from `md`, card rows below it. Closing happens on the month's page, with its checklist. */
function MonthsList({ items, currentMonth }: { items: StatementSummary[]; currentMonth: string }) {
  const { t, locale } = useText()
  return (
    <>
      <div className="hidden overflow-hidden rounded-lg border border-line bg-white md:block">
        <table className="w-full text-sm">
          <caption className="sr-only">{t('peerbonus.admin.statements.months')}</caption>
          <thead className="border-b border-line bg-stone-50 text-left text-xs font-medium text-stone-500">
            <tr>
              <th scope="col" className="px-4 py-2.5">
                {t('peerbonus.admin.col.month')}
              </th>
              <th scope="col" className="px-4 py-2.5">
                {t('peerbonus.admin.statements.col.status')}
              </th>
              <th scope="col" className="px-4 py-2.5 text-right">
                {t('peerbonus.admin.statement.totalPoints')}
              </th>
              <th scope="col" className="px-4 py-2.5 text-right">
                {t('peerbonus.admin.statement.col.amount')}
              </th>
              <th scope="col" className="px-4 py-2.5 text-right">
                {t('peerbonus.admin.statement.memberCount')}
              </th>
              <th scope="col" className="px-4 py-2.5 text-right">
                <span className="sr-only">{t('peerbonus.admin.statements.col.actions')}</span>
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-stone-100">
            {items.map((item) => {
              const label = formatMonth(item.month, locale)
              return (
                <tr key={item.month} className="align-middle">
                  <th scope="row" className="px-4 py-3 text-left font-medium">
                    <HrefLink href={paths.adminStatement(item.month)} className="text-stone-900 hover:underline">
                      {label}
                    </HrefLink>
                  </th>
                  <td className="px-4 py-3">
                    <MonthState item={item} currentMonth={currentMonth} />
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums">{formatCount(item.totalPoints, locale)}</td>
                  {/* The list carries no currency (a closed month keeps the one it was closed with); the detail page shows it. */}
                  <td className="px-4 py-3 text-right tabular-nums">{formatDecimal(item.totalAmount, locale)}</td>
                  <td className="px-4 py-3 text-right tabular-nums">{formatCount(item.memberCount, locale)}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-2">
                      {item.closed ? <CsvLink month={item.month} compact /> : null}
                      <LinkButton size="sm" tone="ghost" href={paths.adminStatement(item.month)} aria-label={t('peerbonus.admin.statements.viewMonth', { month: label })}>
                        {t('peerbonus.admin.statements.view')}
                      </LinkButton>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      <ul aria-label={t('peerbonus.admin.statements.months')} className="divide-y divide-stone-100 rounded-lg border border-line bg-white md:hidden">
        {items.map((item) => {
          const label = formatMonth(item.month, locale)
          return (
            <li key={item.month} className="px-4 py-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <HrefLink href={paths.adminStatement(item.month)} className="inline-flex min-h-10 items-center gap-1 font-medium text-stone-900 hover:underline">
                    {label}
                    <ChevronRight aria-hidden className="size-4 text-stone-400" />
                  </HrefLink>
                  <MonthState item={item} currentMonth={currentMonth} />
                </div>
                {item.closed ? <CsvLink month={item.month} compact /> : null}
              </div>
              <dl className="mt-2 grid grid-cols-3 gap-2 text-xs">
                <div className="min-w-0">
                  <dt className="text-stone-500">{t('peerbonus.admin.statement.totalPoints')}</dt>
                  <dd className="font-semibold text-stone-900 tabular-nums">{formatCount(item.totalPoints, locale)}</dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-stone-500">{t('peerbonus.admin.statement.col.amount')}</dt>
                  <dd className="truncate text-stone-700 tabular-nums">{formatDecimal(item.totalAmount, locale)}</dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-stone-500">{t('peerbonus.admin.statement.memberCount')}</dt>
                  <dd className="text-stone-700 tabular-nums">{formatCount(item.memberCount, locale)}</dd>
                </div>
              </dl>
            </li>
          )
        })}
      </ul>
    </>
  )
}

/**
 * `/admin/statements`: this month's running estimate, then every month from the first points to
 * now (newest first) with its state, totals, its page and, once closed, the CSV.
 */
export function StatementsPage() {
  const { t, locale } = useText()
  const { data: program } = useSuspenseQuery(adminProgramQuery)
  const { data: items } = useSuspenseQuery(statementsQuery)
  const now = useNow(CLOCK_TICK_MS)
  const currentMonth = monthKeyAt(now, program.timeZone)
  const { data: preview } = useSuspenseQuery(statementQuery(currentMonth))
  useDocumentTitle(t('peerbonus.admin.tab.statements'))
  const rate = formatMoney(program.pointUnitValue, program.currency, locale)
  const currentLabel = formatMonth(currentMonth, locale)

  return (
    <div>
      <PageHeader title={t('peerbonus.admin.tab.statements')} lead={t('peerbonus.admin.statements.lead')} />

      <Section
        title={t('peerbonus.admin.statements.preview', { month: currentLabel })}
        lead={t('peerbonus.admin.statements.previewLead', { rate })}
        actions={
          <LinkButton size="sm" href={paths.adminStatement(currentMonth)}>
            {t('peerbonus.admin.statements.viewMonth', { month: currentLabel })}
          </LinkButton>
        }
      >
        {preview.lines.length === 0 ? (
          <p className="rounded-lg border border-dashed border-stone-300 bg-white px-4 py-6 text-center text-sm text-stone-500">{t('peerbonus.admin.statement.empty')}</p>
        ) : (
          <StatementTotals statement={preview} />
        )}
      </Section>

      <Section title={t('peerbonus.admin.statements.months')} lead={t('peerbonus.admin.statements.csvNote')}>
        {items.length === 0 ? (
          <EmptyState
            icon={<CalendarRange />}
            title={t('peerbonus.admin.statements.empty')}
            body={t('peerbonus.admin.statements.emptyBody')}
            action={<LinkButton href={paths.home}>{t('peerbonus.member.openTimeline')}</LinkButton>}
          />
        ) : (
          <MonthsList items={items} currentMonth={currentMonth} />
        )}
      </Section>
    </div>
  )
}
