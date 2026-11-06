import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Send } from 'lucide-react'
import { useState } from 'react'
import type { SlackDelivery, SlackDeliveryState } from '../../../api/peerbonus'
import { useText } from '../../../i18n'
import { paths } from '../../../routes/paths'
import { Chip, formatDateTime, formatRelativeTime, HrefLink, LoadFailed, Loading } from '../../../ui'
import { slackDeliveriesQuery } from '../queries'
import { Card, ToneChip } from './parts'
import { DELIVERY_KIND_LABELS, DELIVERY_STATE_VIEWS, DELIVERY_STATES, SLACK_ERROR_LABELS } from './slack-state'

/** "3 時間前" with the full time on hover and for assistive tech (`<time datetime>`). */
function When({ iso, now }: { iso: string | null; now: number }) {
  const { locale } = useText()
  if (!iso) return <span className="text-stone-400">—</span>
  return (
    <time dateTime={iso} title={formatDateTime(iso, locale)} className="whitespace-nowrap">
      {formatRelativeTime(iso, now, locale)}
    </time>
  )
}

/** The state chip and, under it, why a delivery failed or was skipped. */
function DeliveryState({ delivery }: { delivery: SlackDelivery }) {
  const { t } = useText()
  const view = DELIVERY_STATE_VIEWS[delivery.state]
  const reason = delivery.lastError ? SLACK_ERROR_LABELS[delivery.lastError] : undefined
  return (
    <div className="flex min-w-0 flex-col items-start gap-1">
      <ToneChip tone={view.tone}>{t(view.label)}</ToneChip>
      {delivery.lastError ? <span className="text-xs text-stone-500">{reason ? t(reason) : delivery.lastError}</span> : null}
    </div>
  )
}

/** The post a delivery is about, as a link with the end of its id (ULIDs differ at the end). */
function PostLink({ postId }: { postId: string }) {
  const { t } = useText()
  return (
    <HrefLink href={paths.post(postId)} className="text-stone-700 underline decoration-stone-300 underline-offset-2 hover:text-stone-900 hover:decoration-stone-500">
      {t('peerbonus.post.title')} <span className="font-mono text-xs">…{postId.slice(-6)}</span>
    </HrefLink>
  )
}

/**
 * 配送履歴: the latest deliveries with a state filter; a table from `md`, rows below it. The list
 * refreshes on its own while the page is open, since the worker moves rows from 送信待ち.
 */
export function DeliveriesCard({ now }: { now: number }) {
  const { t } = useText()
  const [state, setState] = useState<SlackDeliveryState | null>(null)
  const deliveries = useQuery({ ...slackDeliveriesQuery(state), placeholderData: keepPreviousData })
  const items = deliveries.data ?? []
  const caption = t('peerbonus.slack.deliveries.title')

  return (
    <Card title={caption} lead={t('peerbonus.slack.deliveries.lead')}>
      <div role="group" aria-label={t('peerbonus.slack.deliveries.filter')} className="mb-3 flex flex-wrap gap-1.5">
        <Chip selected={state === null} onClick={() => setState(null)}>
          {t('peerbonus.slack.deliveries.all')}
        </Chip>
        {DELIVERY_STATES.map((option) => (
          <Chip key={option} selected={state === option} onClick={() => setState(option)}>
            {t(DELIVERY_STATE_VIEWS[option].label)}
          </Chip>
        ))}
      </div>

      {deliveries.isPending ? (
        <Loading />
      ) : deliveries.isError ? (
        <LoadFailed error={deliveries.error} onRetry={() => void deliveries.refetch()} />
      ) : items.length === 0 ? (
        state === null ? (
          <div className="flex flex-col items-center py-6 text-center">
            <Send aria-hidden className="size-5 text-stone-400" />
            <p className="mt-2 text-sm font-medium text-stone-800">{t('peerbonus.slack.deliveries.empty')}</p>
            <p className="mt-1 max-w-sm text-sm text-stone-500">{t('peerbonus.slack.deliveries.emptyBody')}</p>
          </div>
        ) : (
          <p className="py-6 text-center text-sm text-stone-500">{t('peerbonus.slack.deliveries.emptyFiltered')}</p>
        )
      ) : (
        <>
          <table className="hidden w-full text-sm md:table">
            <caption className="sr-only">{caption}</caption>
            <thead className="border-b border-line text-left text-xs font-medium text-stone-500">
              <tr>
                <th scope="col" className="py-2 pr-4">
                  {t('peerbonus.slack.deliveries.col.kind')}
                </th>
                <th scope="col" className="py-2 pr-4">
                  {t('peerbonus.slack.deliveries.col.state')}
                </th>
                <th scope="col" className="py-2 pr-4 text-right">
                  {t('peerbonus.slack.deliveries.col.attempts')}
                </th>
                <th scope="col" className="py-2 pr-4">
                  {t('peerbonus.slack.deliveries.col.created')}
                </th>
                <th scope="col" className="py-2">
                  {t('peerbonus.slack.deliveries.col.sent')}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-stone-100">
              {items.map((delivery) => (
                <tr key={delivery.id} className="align-top">
                  <th scope="row" className="py-2.5 pr-4 text-left font-normal">
                    <span className="block font-medium text-stone-900">{t(DELIVERY_KIND_LABELS[delivery.kind])}</span>
                    <PostLink postId={delivery.postId} />
                  </th>
                  <td className="py-2.5 pr-4">
                    <DeliveryState delivery={delivery} />
                  </td>
                  <td className="py-2.5 pr-4 text-right tabular-nums">{delivery.attempts}</td>
                  <td className="py-2.5 pr-4 text-stone-600">
                    <When iso={delivery.createdAt} now={now} />
                  </td>
                  <td className="py-2.5 text-stone-600">
                    <When iso={delivery.sentAt} now={now} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <ul aria-label={caption} className="divide-y divide-stone-100 border-t border-line md:hidden">
            {items.map((delivery) => (
              <li key={delivery.id} className="flex items-start justify-between gap-3 py-3">
                <div className="min-w-0 text-sm">
                  <p className="font-medium text-stone-900">{t(DELIVERY_KIND_LABELS[delivery.kind])}</p>
                  <p className="mt-0.5 text-xs text-stone-500">
                    <PostLink postId={delivery.postId} /> · <When iso={delivery.sentAt ?? delivery.createdAt} now={now} /> ·{' '}
                    {t('peerbonus.slack.deliveries.attempts', { count: delivery.attempts })}
                  </p>
                </div>
                <div className="shrink-0">
                  <DeliveryState delivery={delivery} />
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </Card>
  )
}
