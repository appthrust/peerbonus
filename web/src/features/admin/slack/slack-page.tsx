import { useMutation, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { Check, ChevronDown, Download, ExternalLink, RefreshCw, ShieldCheck } from 'lucide-react'
import { useId, useState } from 'react'
import { peerbonusApi, slackTeamChange, type SlackAdmin, type SlackTeamChange } from '../../../api/peerbonus'
import { configQuery } from '../../../api/queries'
import type { MessageId } from '../../../copy'
import { useText } from '../../../i18n'
import { Button, buttonClasses, CLOCK_TICK_MS, cn, ConfirmDialog, formatDateTime, Notice, PageHeader, useDocumentTitle, useNow } from '../../../ui'
import { formatCount } from '../format'
import { slackQuery, SLACK_KEY } from '../queries'
import { DeliveriesCard } from './deliveries'
import { generateManifest, manifestHref } from './manifest'
import { Card, CopyField, ToneChip } from './parts'
import { SlackActionError } from './slack-error'
import { SlackSettingsCard } from './slack-settings'
import { SLACK_ERROR_LABELS, setupSteps, STATE_VIEWS, type SetupStepId } from './slack-state'

const SLACK_APPS_URL = 'https://api.slack.com/apps'
const MANIFEST_FILE = 'peerbonus-slack-manifest.json'

/**
 * `/admin/slack`: one card per concern — the connection (state, workspace, verify), the setup guide
 * (folded once done), delivery settings, the Slack inbound endpoint (only once the platform projected
 * it, Phase B) and the delivery log.
 */
export function SlackPage() {
  const { t } = useText()
  const { data: slack } = useSuspenseQuery(slackQuery)
  const { data: config } = useSuspenseQuery(configQuery)
  const now = useNow(CLOCK_TICK_MS)
  // Which stored settings a save returned: the remounted form shows its saved notice for them.
  const [savedSettings, setSavedSettings] = useState<string | null>(null)
  const settingsKey = JSON.stringify(slack.settings)
  useDocumentTitle(t('peerbonus.slack.title'))

  return (
    <div>
      <PageHeader title={t('peerbonus.slack.title')} lead={t('peerbonus.slack.lead')} />
      <div className="space-y-4">
        <StatusCard slack={slack} />
        <SetupGuide slack={slack} projectId={config.projectId} />
        <SlackSettingsCard key={settingsKey} slack={slack} justSaved={savedSettings === settingsKey} onSaved={setSavedSettings} />
        {slack.inbound ? <InboundCard inbound={slack.inbound} /> : null}
        <DeliveriesCard now={now} />
      </div>
    </div>
  )
}

/** 接続: the state chip, the fixed workspace, delivery counts, the last error and the verify action. */
function StatusCard({ slack }: { slack: SlackAdmin }) {
  const { t, locale } = useText()
  const client = useQueryClient()
  const view = STATE_VIEWS[slack.state]
  // The workspaces of a refused verify; set when the switch dialog is open.
  const [switching, setSwitching] = useState<SlackTeamChange | 'unknown' | null>(null)

  const store = async (result: SlackAdmin) => {
    client.setQueryData(slackQuery.queryKey, result)
    await client.invalidateQueries({ queryKey: SLACK_KEY })
  }
  const verify = useMutation({
    mutationFn: () => peerbonusApi.admin.slack.verify(),
    onSuccess: store,
    onError: (error) => {
      const change = slackTeamChange(error)
      if (change) setSwitching(change)
    },
  })
  const force = useMutation({
    mutationFn: () => peerbonusApi.admin.slack.verify(true),
    onSuccess: async (result) => {
      setSwitching(null)
      verify.reset()
      await store(result)
    },
  })

  const lastError = slack.lastError
  const lastErrorLabel = lastError ? SLACK_ERROR_LABELS[lastError.code] : undefined
  const verified = verify.isSuccess || force.isSuccess
  const action =
    view.action === 'verify' || view.action === 'reverify' ? (
      <Button
        tone={view.action === 'verify' ? 'primary' : 'secondary'}
        icon={view.action === 'verify' ? <ShieldCheck /> : <RefreshCw />}
        loading={verify.isPending}
        onClick={() => {
          force.reset()
          verify.mutate()
        }}
      >
        {t(view.action === 'verify' ? 'peerbonus.slack.verify' : 'peerbonus.slack.reverify')}
      </Button>
    ) : view.action === 'force' ? (
      <Button tone="danger" icon={<RefreshCw />} onClick={() => setSwitching('unknown')}>
        {t('peerbonus.slack.forceVerify')}
      </Button>
    ) : null

  return (
    <Card title={t('peerbonus.slack.status.title')} actions={<ToneChip tone={view.tone}>{t(view.label)}</ToneChip>}>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          {slack.workspace ? (
            <>
              <p className="text-lg font-semibold break-words text-stone-900">{slack.workspace.teamName}</p>
              <p className="text-xs text-stone-500">
                <span className="font-mono">{slack.workspace.teamId}</span> · {t('peerbonus.slack.status.verifiedAt', { date: formatDateTime(slack.workspace.verifiedAt, locale) })}
              </p>
            </>
          ) : null}
          <p className={cn('max-w-prose text-sm text-stone-600', slack.workspace ? 'mt-2' : undefined)}>{t(view.body)}</p>
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>

      {slack.workspace ? (
        <dl className="mt-4 flex flex-wrap gap-x-8 gap-y-2 border-t border-stone-100 pt-4 text-sm">
          {(
            [
              ['peerbonus.slack.deliveries.state.pending', slack.deliveries.pending],
              ['peerbonus.slack.status.sent24h', slack.deliveries.sent24h],
              ['peerbonus.slack.status.failed24h', slack.deliveries.failed24h],
            ] as const
          ).map(([label, count]) => (
            <div key={label}>
              <dt className="text-xs text-stone-500">{t(label)}</dt>
              <dd className={cn('text-lg font-semibold tabular-nums', label === 'peerbonus.slack.status.failed24h' && count > 0 ? 'text-bad-ink' : 'text-stone-900')}>
                {formatCount(count, locale)}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}

      {lastError && lastError.code !== 'team_changed' ? (
        <Notice tone="warning" title={t('peerbonus.slack.status.lastError', { date: formatDateTime(lastError.at, locale) })} className="mt-4">
          {lastErrorLabel ? t(lastErrorLabel) : <span className="font-mono">{lastError.code}</span>}
        </Notice>
      ) : null}
      {verify.isError && !slackTeamChange(verify.error) ? <SlackActionError error={verify.error} className="mt-4" /> : null}
      {verified && slack.workspace ? (
        <Notice tone="success" className="mt-4">
          {t('peerbonus.slack.verified', { team: slack.workspace.teamName })}
        </Notice>
      ) : null}

      <ConfirmDialog
        open={switching !== null}
        onOpenChange={(open) => {
          if (open) return
          setSwitching(null)
          force.reset()
        }}
        title={t('peerbonus.slack.force.title')}
        body={t('peerbonus.slack.force.body', {
          current: switching !== null && switching !== 'unknown' ? switching.current.teamName : (slack.workspace?.teamName ?? ''),
          next: switching !== null && switching !== 'unknown' ? switching.token.teamName : t('peerbonus.slack.force.tokenWorkspace'),
        })}
        confirmLabel={t('peerbonus.slack.force.confirm')}
        tone="danger"
        error={force.error ?? undefined}
        onConfirm={() => force.mutate()}
      />
    </Card>
  )
}

const STEP_COPY: Record<SetupStepId, { title: MessageId; body: MessageId }> = {
  createApp: { title: 'peerbonus.slack.setup.createApp', body: 'peerbonus.slack.setup.createAppBody' },
  pasteToken: { title: 'peerbonus.slack.setup.pasteToken', body: 'peerbonus.slack.setup.pasteTokenBody' },
  connect: { title: 'peerbonus.slack.setup.connect', body: 'peerbonus.slack.setup.connectBody' },
  signingSecret: { title: 'peerbonus.slack.setup.signingSecret', body: 'peerbonus.slack.setup.signingSecretBody' },
}

/**
 * セットアップ手順 (design §8), with the steps the read model proves done checked off. Folded once
 * every step is done. The Dashboard's origin is unknown here, so the credentials page is given as a
 * path to open on the Dashboard the admin already uses.
 */
function SetupGuide({ slack, projectId }: { slack: SlackAdmin; projectId: string }) {
  const { t } = useText()
  const id = useId()
  const steps = setupSteps(slack)
  const complete = steps.every((step) => step.done)
  const [open, setOpen] = useState(!complete)
  const inboundUrl = slack.inbound?.url ?? null
  const manifest = generateManifest({ inboundUrl, phaseB: inboundUrl !== null })
  const dashboardPath = `/dashboard/projects/${encodeURIComponent(projectId)}/marketplace/peerbonus/credentials`

  return (
    <Card
      title={t('peerbonus.slack.setup.title')}
      lead={complete ? t('peerbonus.slack.setup.complete') : t('peerbonus.slack.setup.lead')}
      actions={
        complete ? (
          <Button
            size="sm"
            tone="ghost"
            aria-expanded={open}
            onClick={() => setOpen(!open)}
            icon={<ChevronDown className={cn('transition-transform', open ? 'rotate-180' : undefined)} />}
          >
            {t(open ? 'peerbonus.slack.setup.hide' : 'peerbonus.slack.setup.show')}
          </Button>
        ) : null
      }
    >
      {open ? (
        <ol className="space-y-5">
          {steps.map((step, index) => (
            <li key={step.id} className="flex gap-3">
              <span
                aria-hidden
                className={cn(
                  'mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold',
                  step.done ? 'bg-ok text-white' : 'bg-stone-100 text-stone-600 ring-1 ring-stone-200 ring-inset'
                )}
              >
                {step.done ? <Check className="size-3.5" /> : index + 1}
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-stone-900">
                  {t(STEP_COPY[step.id].title)}
                  {step.done ? <span className="sr-only"> ({t('peerbonus.slack.setup.done')})</span> : null}
                </p>
                <p className="mt-0.5 max-w-prose text-sm text-stone-500">{t(STEP_COPY[step.id].body)}</p>
                {step.id === 'createApp' ? (
                  <div className="mt-2 flex flex-wrap gap-2">
                    <a href={SLACK_APPS_URL} target="_blank" rel="noreferrer" className={buttonClasses('secondary', 'sm')}>
                      <ExternalLink aria-hidden />
                      {t('peerbonus.slack.setup.openSlackApps')}
                    </a>
                    <a href={manifestHref(manifest)} download={MANIFEST_FILE} className={buttonClasses('secondary', 'sm')}>
                      <Download aria-hidden />
                      {t('peerbonus.slack.setup.downloadManifest')}
                    </a>
                  </div>
                ) : null}
                {step.id === 'pasteToken' ? (
                  <div className="mt-2 max-w-xl">
                    <CopyField id={`${id}-dashboard`} value={dashboardPath} label={t('peerbonus.slack.setup.dashboardPath')} />
                    <p className="mt-1.5 text-xs text-stone-500">{t('peerbonus.slack.setup.dashboardPathHint')}</p>
                  </div>
                ) : null}
              </div>
            </li>
          ))}
        </ol>
      ) : null}
    </Card>
  )
}

/** Slack からのリクエスト (Phase B): the projected endpoint's URL to copy, its readiness, the full manifest. */
function InboundCard({ inbound }: { inbound: NonNullable<SlackAdmin['inbound']> }) {
  const { t } = useText()
  const id = useId()
  return (
    <Card
      title={t('peerbonus.slack.inbound.title')}
      lead={t('peerbonus.slack.inbound.lead')}
      actions={<ToneChip tone={inbound.ready ? 'ok' : 'busy'}>{t(inbound.ready ? 'peerbonus.slack.inbound.ready' : 'peerbonus.slack.inbound.notReady')}</ToneChip>}
    >
      {inbound.url ? (
        <div className="max-w-xl">
          <label htmlFor={`${id}-url`} className="text-sm font-medium text-stone-800">
            {t('peerbonus.slack.inbound.url')}
          </label>
          <div className="mt-1.5">
            <CopyField id={`${id}-url`} value={inbound.url} label={t('peerbonus.slack.inbound.url')} />
          </div>
        </div>
      ) : (
        <p className="text-sm text-stone-600">{t('peerbonus.slack.inbound.noUrl')}</p>
      )}
      {inbound.reason && !inbound.ready ? <p className="mt-2 text-xs text-stone-500">{t('peerbonus.slack.inbound.reason', { reason: inbound.reason })}</p> : null}
      {inbound.url ? (
        <div className="mt-4">
          <a href={manifestHref(generateManifest({ inboundUrl: inbound.url, phaseB: true }))} download={MANIFEST_FILE} className={buttonClasses('secondary', 'md')}>
            <Download aria-hidden />
            {t('peerbonus.slack.inbound.download')}
          </a>
          <p className="mt-1.5 text-xs text-stone-500">{t('peerbonus.slack.inbound.downloadHint')}</p>
        </div>
      ) : null}
    </Card>
  )
}
