import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Lock } from 'lucide-react'
import { useState } from 'react'
import { isPeerbonusError, peerbonusApi, type Statement } from '../../api/peerbonus'
import { invalidatePeerbonus } from '../../api/queries'
import type { MessageId } from '../../copy'
import { useText } from '../../i18n'
import { Button, ConfirmDialog, Notice } from '../../ui'
import { formatMonth } from './format'
import { statementQuery } from './queries'

/** The 409/400 answers a close can get, which the dialog explains itself. */
const CLOSE_ERRORS: Record<string, MessageId> = {
  month_not_ended: 'peerbonus.admin.statements.error.monthNotEnded',
  previous_month_open: 'peerbonus.admin.statements.error.previousMonthOpen',
  invalid_month: 'peerbonus.admin.statements.error.invalidMonth',
}

/**
 * 締める for one month: enabled only when the close checklist is all ✓ (`closable`; the checklist,
 * `describedBy`, says what is missing), then a confirmation that names the rate the statement freezes at.
 */
export function CloseStatement({
  month,
  closable,
  rate,
  onClosed,
  describedBy,
}: {
  month: string
  closable: boolean
  /** The current rate ("¥1 / pt" formatted), which the close freezes. */
  rate: string
  onClosed: (statement: Statement) => void
  /** The id of the checklist that explains a disabled button. */
  describedBy?: string
}) {
  const { t, locale } = useText()
  const client = useQueryClient()
  const [open, setOpen] = useState(false)
  const monthLabel = formatMonth(month, locale)
  const mutation = useMutation({
    mutationFn: () => peerbonusApi.admin.closeStatement(month),
    onSuccess: async (statement) => {
      setOpen(false)
      client.setQueryData(statementQuery(month).queryKey, statement)
      onClosed(statement)
      await invalidatePeerbonus(client)
    },
    onError: async (error) => {
      // The page was stale (someone else closed or the clock moved): show the server's view.
      if (isPeerbonusError(error, 'previous_month_open') || isPeerbonusError(error, 'month_not_ended')) await invalidatePeerbonus(client)
    },
  })
  const known = isPeerbonusError(mutation.error) ? CLOSE_ERRORS[mutation.error.code] : undefined

  return (
    <>
      <Button
        tone={closable ? 'primary' : 'secondary'}
        icon={<Lock />}
        disabled={!closable}
        aria-describedby={describedBy}
        aria-label={t('peerbonus.admin.statements.closeMonth', { month: monthLabel })}
        onClick={() => setOpen(true)}
      >
        {t('peerbonus.admin.statements.close')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) mutation.reset()
        }}
        title={t('peerbonus.admin.statements.confirmTitle', { month: monthLabel })}
        body={
          <>
            <p>{t('peerbonus.admin.statements.confirmBody', { month: monthLabel, rate })}</p>
            {known ? (
              <Notice tone="error" className="mt-3">
                {t(known)}
              </Notice>
            ) : null}
          </>
        }
        confirmLabel={t('peerbonus.admin.statements.close')}
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={known ? undefined : (mutation.error ?? undefined)}
      />
    </>
  )
}
