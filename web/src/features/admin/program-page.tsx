import { useMutation, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { Lock, Save } from 'lucide-react'
import { useId, useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { isPeerbonusError, peerbonusApi, type ProgramAdmin } from '../../api/peerbonus'
import { invalidatePeerbonus } from '../../api/queries'
import { useText } from '../../i18n'
import { ActionError, Button, cn, Field, formatDateTime, Notice, PageHeader, Section, TextInput, useDocumentTitle } from '../../ui'
import { draftOf, hasProblems, isDirty, MAX_WEEKLY_ALLOWANCE, patchOf, validateProgram, type ProgramDraft } from './program-form'
import { adminProgramQuery } from './queries'
import { ValuesEditor } from './values-editor'

/** IANA zones the browser knows, for the time zone suggestions. */
const TIME_ZONES: readonly string[] = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : []

/** One group of settings: a titled card whose fields sit side by side from `sm`. */
function SettingsCard({ title, lead, className, children }: { title: string; lead: string; className?: string; children: ReactNode }) {
  const headingId = useId()
  return (
    <section aria-labelledby={headingId} className={cn('rounded-lg border border-line bg-white p-4 sm:p-5', className)}>
      <h2 id={headingId} className="text-base font-semibold text-stone-900">
        {title}
      </h2>
      <p className="mt-1 text-sm text-stone-500">{lead}</p>
      <div className="mt-4 grid gap-4 sm:grid-cols-2">{children}</div>
    </section>
  )
}

/** `/admin/program`: three settings cards (週の配布, 支払い換算, タイムゾーン), the values, and a sticky save bar. */
export function ProgramPage() {
  const { data: program } = useSuspenseQuery(adminProgramQuery)
  // The `updatedAt` a save returned: the remounted form shows the saved notice for it.
  const [savedAt, setSavedAt] = useState<string | null>(null)
  // A save replaces `program`; the form restarts from what the server stored.
  return <ProgramForm key={program.updatedAt} program={program} justSaved={savedAt === program.updatedAt} onSaved={setSavedAt} />
}

function ProgramForm({ program, justSaved, onSaved }: { program: ProgramAdmin; justSaved: boolean; onSaved: (updatedAt: string) => void }) {
  const { t, locale } = useText()
  const client = useQueryClient()
  const id = useId()
  const [draft, setDraft] = useState<ProgramDraft>(() => draftOf(program))
  // Errors show after the first save attempt and then follow every edit.
  const [attempted, setAttempted] = useState(false)
  const [saved, setSaved] = useState(justSaved)
  useDocumentTitle(t('peerbonus.admin.program.title'))

  const problems = useMemo(() => validateProgram(draft, { timeZoneLocked: program.timeZoneLocked }), [draft, program.timeZoneLocked])
  const shown = attempted ? problems : { values: {} }
  const dirty = isDirty(draft, program)

  const mutation = useMutation({
    mutationFn: () => peerbonusApi.admin.updateProgram(patchOf(draft, program)),
    onSuccess: async (result) => {
      // The new `updatedAt` remounts the form with the stored values.
      onSaved(result.updatedAt)
      client.setQueryData(adminProgramQuery.queryKey, result)
      await invalidatePeerbonus(client)
    },
    onError: async (error) => {
      if (!isPeerbonusError(error, 'timezone_locked')) return
      // Someone recorded points since this page loaded: reload (the zone shows as locked) and put the stored zone back.
      await client.invalidateQueries({ queryKey: adminProgramQuery.queryKey })
      const stored = client.getQueryData(adminProgramQuery.queryKey)
      if (stored) setDraft((current) => ({ ...current, timeZone: stored.timeZone }))
    },
  })

  useBlocker({
    shouldBlockFn: () => dirty && !mutation.isPending && !window.confirm(t('peerbonus.admin.program.leave')),
    enableBeforeUnload: () => dirty,
  })

  const edit = (change: Partial<ProgramDraft>) => {
    setDraft((current) => ({ ...current, ...change }))
    setSaved(false)
    if (mutation.isError) mutation.reset()
  }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    setAttempted(true)
    setSaved(false)
    if (hasProblems(problems) || !dirty) return
    mutation.mutate()
  }
  const discard = () => {
    setDraft(draftOf(program))
    setAttempted(false)
    mutation.reset()
  }

  const allowance = Number(draft.weeklyAllowance)
  const maxBound = Number.isInteger(allowance) && allowance >= 1 && allowance <= MAX_WEEKLY_ALLOWANCE ? allowance : MAX_WEEKLY_ALLOWANCE

  return (
    <form noValidate onSubmit={submit} aria-describedby={attempted && hasProblems(problems) ? `${id}-problems` : undefined}>
      <PageHeader
        title={t('peerbonus.admin.program.title')}
        lead={
          <>
            {t('peerbonus.admin.program.lead')}{' '}
            <span className="whitespace-nowrap">{t('peerbonus.admin.program.updatedAt', { date: formatDateTime(program.updatedAt, locale) })}</span>
          </>
        }
      />

      <div className="grid gap-4 lg:grid-cols-2">
        <SettingsCard title={t('peerbonus.admin.program.allowanceSection')} lead={t('peerbonus.admin.program.allowanceLead')}>
          <Field
            id={`${id}-allowance`}
            label={t('peerbonus.admin.program.weeklyAllowance')}
            hint={t('peerbonus.admin.program.weeklyAllowanceHint')}
            error={shown.weeklyAllowance ? t('peerbonus.admin.program.error.weeklyAllowance') : undefined}
          >
            <TextInput
              id={`${id}-allowance`}
              type="number"
              inputMode="numeric"
              min={0}
              max={MAX_WEEKLY_ALLOWANCE}
              step={1}
              value={draft.weeklyAllowance}
              invalid={Boolean(shown.weeklyAllowance)}
              className="tabular-nums"
              onChange={(event) => edit({ weeklyAllowance: event.target.value })}
            />
          </Field>
          <Field
            id={`${id}-max`}
            label={t('peerbonus.admin.program.maxPointsPerPost')}
            hint={t('peerbonus.admin.program.maxPointsPerPostHint', { max: maxBound })}
            error={
              shown.maxPointsPerPost === 'noAllowance'
                ? t('peerbonus.admin.program.error.maxPointsNoAllowance')
                : shown.maxPointsPerPost
                  ? t('peerbonus.admin.program.error.maxPointsPerPost', { max: maxBound })
                  : undefined
            }
          >
            <TextInput
              id={`${id}-max`}
              type="number"
              inputMode="numeric"
              min={1}
              max={maxBound}
              step={1}
              value={draft.maxPointsPerPost}
              invalid={Boolean(shown.maxPointsPerPost)}
              className="tabular-nums"
              onChange={(event) => edit({ maxPointsPerPost: event.target.value })}
            />
          </Field>
        </SettingsCard>

        <SettingsCard title={t('peerbonus.admin.program.payoutSection')} lead={t('peerbonus.admin.program.payoutLead')}>
          <Field
            id={`${id}-unit`}
            label={t('peerbonus.admin.program.pointUnitValue')}
            hint={t('peerbonus.admin.program.pointUnitValueHint')}
            error={shown.pointUnitValue ? t('peerbonus.admin.program.error.pointUnitValue') : undefined}
          >
            <TextInput
              id={`${id}-unit`}
              inputMode="decimal"
              autoComplete="off"
              value={draft.pointUnitValue}
              invalid={Boolean(shown.pointUnitValue)}
              className="tabular-nums"
              onChange={(event) => edit({ pointUnitValue: event.target.value })}
            />
          </Field>
          <Field
            id={`${id}-currency`}
            label={t('peerbonus.admin.program.currency')}
            hint={t('peerbonus.admin.program.currencyHint')}
            error={shown.currency ? t('peerbonus.admin.program.error.currency') : undefined}
          >
            <TextInput
              id={`${id}-currency`}
              value={draft.currency}
              maxLength={3}
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
              invalid={Boolean(shown.currency)}
              className="font-mono uppercase"
              onChange={(event) => edit({ currency: event.target.value.toUpperCase() })}
            />
          </Field>
        </SettingsCard>

        <SettingsCard title={t('peerbonus.admin.program.timeZoneSection')} lead={t('peerbonus.admin.program.timeZoneLead')} className="lg:col-span-2">
          <Field
            id={`${id}-tz`}
            label={t('peerbonus.admin.program.timeZone')}
            hint={
              program.timeZoneLocked ? (
                <span className="flex gap-1.5">
                  <Lock aria-hidden className="mt-0.5 size-3.5 shrink-0" />
                  {t('peerbonus.admin.program.timeZoneLocked')}
                </span>
              ) : (
                t('peerbonus.admin.program.timeZoneHint')
              )
            }
            error={shown.timeZone ? t('peerbonus.admin.program.error.timeZone') : undefined}
          >
            <TextInput
              id={`${id}-tz`}
              value={draft.timeZone}
              disabled={program.timeZoneLocked}
              invalid={Boolean(shown.timeZone)}
              list={program.timeZoneLocked ? undefined : `${id}-tz-list`}
              autoCapitalize="none"
              autoComplete="off"
              spellCheck={false}
              className="sm:max-w-sm"
              onChange={(event) => edit({ timeZone: event.target.value })}
            />
            {program.timeZoneLocked ? null : (
              <datalist id={`${id}-tz-list`}>
                {TIME_ZONES.map((zone) => (
                  <option key={zone} value={zone} />
                ))}
              </datalist>
            )}
          </Field>
        </SettingsCard>
      </div>

      <Section title={t('peerbonus.compose.values')} lead={t('peerbonus.admin.program.valuesLead')}>
        <ValuesEditor id={`${id}-values`} rows={draft.values} problems={shown.values} onChange={(values) => edit({ values })} />
      </Section>

      <div className="sticky bottom-0 z-10 -mx-4 mt-8 border-t border-stone-200 bg-white/95 px-4 py-3 backdrop-blur sm:mx-0 sm:rounded-lg sm:border">
        <div className="space-y-3">
          {attempted && hasProblems(problems) ? (
            <Notice tone="error" className="py-2">
              <span id={`${id}-problems`}>{t('peerbonus.admin.program.fixErrors')}</span>
            </Notice>
          ) : null}
          {mutation.isError ? (
            isPeerbonusError(mutation.error, 'timezone_locked') ? (
              <Notice tone="error" className="py-2">
                {t('peerbonus.admin.program.timezoneLockedError')}
              </Notice>
            ) : (
              <ActionError error={mutation.error} className="py-2" />
            )
          ) : null}
          {saved && !dirty ? (
            <Notice tone="success" className="py-2">
              {t('peerbonus.admin.program.saved')}
            </Notice>
          ) : null}
          <div className="flex flex-wrap items-center justify-end gap-2">
            <span className="mr-auto text-xs text-stone-500" aria-live="polite">
              {dirty ? t('peerbonus.admin.program.unsaved') : null}
            </span>
            <Button disabled={!dirty || mutation.isPending} onClick={discard}>
              {t('peerbonus.admin.program.discard')}
            </Button>
            <Button type="submit" tone="primary" icon={<Save />} loading={mutation.isPending} disabled={!dirty}>
              {t('peerbonus.admin.program.save')}
            </Button>
          </div>
        </div>
      </div>
    </form>
  )
}
