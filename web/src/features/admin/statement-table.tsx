import type { Statement, StatementLine } from '../../api/peerbonus'
import { useText } from '../../i18n'
import { paths } from '../../routes/paths'
import { Avatar, HrefLink, StatCard } from '../../ui'
import { PendingMark } from '../members/profile-parts'
import { formatCount, formatMoney } from './format'

/** Avatar, linked name, the quiet pending mark and the email of one statement line. */
function LinePerson({ line }: { line: StatementLine }) {
  const { t } = useText()
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <Avatar name={line.member.displayName} avatarUrl={line.member.avatarUrl} size="md" />
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-1">
          <HrefLink href={paths.member(line.member.id)} className="truncate font-medium text-stone-900 hover:underline">
            {line.member.displayName}
          </HrefLink>
          {line.member.pending ? <PendingMark hint={t('peerbonus.admin.pendingHint')} /> : null}
        </div>
        <div className="truncate text-xs text-stone-500">{line.member.email}</div>
      </div>
    </div>
  )
}

/**
 * One statement's lines (name, email, pending, points, amount) with the total: a table from `md`,
 * card rows below it, so nothing scrolls sideways on a phone (principle 9).
 */
export function StatementTable({ statement, caption }: { statement: Statement; caption: string }) {
  const { t, locale } = useText()
  const total = (
    <>
      {t('peerbonus.admin.statement.total')}
      <span className="ml-2 text-xs font-normal text-stone-500">{t('peerbonus.admin.statement.people', { count: statement.memberCount })}</span>
    </>
  )
  return (
    <>
      <div className="hidden overflow-hidden rounded-lg border border-line bg-white md:block">
        <table className="w-full text-sm">
          <caption className="sr-only">{caption}</caption>
          <thead className="border-b border-line bg-stone-50 text-left text-xs font-medium text-stone-500">
            <tr>
              <th scope="col" className="px-4 py-2.5">
                {t('peerbonus.admin.col.name')}
              </th>
              <th scope="col" className="px-4 py-2.5 text-right">
                {t('peerbonus.admin.col.receivedPoints')}
              </th>
              <th scope="col" className="px-4 py-2.5 text-right">
                {t('peerbonus.admin.statement.col.amount')}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-stone-100">
            {statement.lines.map((line) => (
              <tr key={line.member.id}>
                <th scope="row" className="max-w-96 px-4 py-2.5 text-left font-normal">
                  <LinePerson line={line} />
                </th>
                <td className="px-4 py-2.5 text-right tabular-nums">{formatCount(line.points, locale)}</td>
                <td className="px-4 py-2.5 text-right tabular-nums">{formatMoney(line.amount, statement.currency, locale)}</td>
              </tr>
            ))}
          </tbody>
          <tfoot className="border-t border-line bg-stone-50 font-medium text-stone-900">
            <tr>
              <th scope="row" className="px-4 py-2.5 text-left">
                {total}
              </th>
              <td className="px-4 py-2.5 text-right tabular-nums">{formatCount(statement.totalPoints, locale)}</td>
              <td className="px-4 py-2.5 text-right tabular-nums">{formatMoney(statement.totalAmount, statement.currency, locale)}</td>
            </tr>
          </tfoot>
        </table>
      </div>

      <div className="rounded-lg border border-line bg-white md:hidden">
        <ul aria-label={caption} className="divide-y divide-stone-100">
          {statement.lines.map((line) => (
            <li key={line.member.id} className="flex items-center justify-between gap-3 px-4 py-3">
              <LinePerson line={line} />
              <div className="shrink-0 text-right text-sm tabular-nums">
                <div className="font-semibold text-stone-900">{t('peerbonus.common.points', { points: formatCount(line.points, locale) })}</div>
                <div className="text-xs text-stone-500">{formatMoney(line.amount, statement.currency, locale)}</div>
              </div>
            </li>
          ))}
        </ul>
        <div className="flex items-center justify-between gap-3 border-t border-line bg-stone-50 px-4 py-3 text-sm font-medium text-stone-900">
          <span>{total}</span>
          <span className="shrink-0 text-right tabular-nums">
            <span className="block">{t('peerbonus.common.points', { points: formatCount(statement.totalPoints, locale) })}</span>
            <span className="block text-xs font-normal text-stone-500">{formatMoney(statement.totalAmount, statement.currency, locale)}</span>
          </span>
        </div>
      </div>
    </>
  )
}

/** Totals as a row of stat cards above a statement (or the current month's preview). */
export function StatementTotals({ statement }: { statement: Statement }) {
  const { t, locale } = useText()
  return (
    <dl className="grid grid-cols-2 gap-2 sm:gap-3 lg:grid-cols-4">
      <StatCard label={t('peerbonus.admin.statement.totalPoints')} value={formatCount(statement.totalPoints, locale)} unit={t('peerbonus.common.pointsUnit')} />
      <StatCard label={t('peerbonus.admin.statement.totalAmount')} value={formatMoney(statement.totalAmount, statement.currency, locale)} />
      <StatCard label={t('peerbonus.admin.statement.memberCount')} value={formatCount(statement.memberCount, locale)} unit={t('peerbonus.admin.participation.peopleUnit')} />
      <StatCard
        label={t('peerbonus.admin.statement.rate')}
        value={formatMoney(statement.pointUnitValue, statement.currency, locale)}
        unit={t('peerbonus.admin.statement.perPoint')}
      />
    </dl>
  )
}
