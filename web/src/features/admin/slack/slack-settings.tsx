import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { Lock, Save, Send } from 'lucide-react'
import { useId, useReducer, useState, type FormEvent } from 'react'
import { peerbonusApi, type SlackAdmin, type SlackLocale, type SlackSettings } from '../../../api/peerbonus'
import { LANGUAGES, useText } from '../../../i18n'
import { Button, cn, controlClasses, Notice } from '../../../ui'
import { slackQuery, SLACK_KEY } from '../queries'
import { ChannelPicker } from './channel-picker'
import { Card, SwitchRow } from './parts'
import { SlackActionError } from './slack-error'
import { slackDraftReducer, slackPatchOf, validateSlackDraft } from './slack-form'

/**
 * 配信設定: the announce channel, the three switches and the message language, saved together, and
 * the test message (to the saved channel). Locked until the workspace is connected. A save replaces
 * the settings; the form restarts from what the server stored (`key` by the caller).
 */
export function SlackSettingsCard({ slack, justSaved, onSaved }: { slack: SlackAdmin; justSaved: boolean; onSaved: (settings: SlackSettings) => void }) {
  const { t } = useText()
  const client = useQueryClient()
  const id = useId()
  const [draft, dispatch] = useReducer(slackDraftReducer, slack.settings)
  const [saved, setSaved] = useState(justSaved)
  const locked = slack.state !== 'connected'
  const problems = validateSlackDraft(draft, { signingSecret: slack.credential.signingSecret })
  const patch = slackPatchOf(draft, slack.settings)
  const dirty = Object.keys(patch).length > 0
  const hasProblems = Object.keys(problems).length > 0
  const savedChannel = slack.settings.announceChannel

  const save = useMutation({
    mutationFn: () => peerbonusApi.admin.slack.update(patch),
    onSuccess: async (result) => {
      onSaved(result.settings)
      client.setQueryData(slackQuery.queryKey, result)
      await client.invalidateQueries({ queryKey: SLACK_KEY })
    },
  })
  const test = useMutation({
    mutationFn: () => peerbonusApi.admin.slack.test(),
    onSettled: () => client.invalidateQueries({ queryKey: SLACK_KEY }),
  })

  useBlocker({
    shouldBlockFn: () => dirty && !save.isPending && !window.confirm(t('peerbonus.admin.program.leave')),
    enableBeforeUnload: () => dirty,
  })

  const edit = (action: Parameters<typeof dispatch>[0]) => {
    dispatch(action)
    setSaved(false)
    if (save.isError) save.reset()
    if (!test.isIdle) test.reset()
  }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!dirty || hasProblems || locked) return
    save.mutate()
  }

  return (
    <Card title={t('peerbonus.slack.settings.title')} lead={t('peerbonus.slack.settings.lead')}>
      <form noValidate onSubmit={submit}>
        {locked ? (
          <p className="mb-4 flex items-center gap-2 text-sm text-stone-600">
            <Lock aria-hidden className="size-4 shrink-0 text-stone-400" />
            {t('peerbonus.slack.settings.locked')}
          </p>
        ) : null}
        <fieldset disabled={locked} className="min-w-0">
          <div>
            <p id={`${id}-channel-label`} className="text-sm font-medium text-stone-800">
              {t('peerbonus.slack.channel')}
            </p>
            <div className="mt-1.5">
              <ChannelPicker
                id={`${id}-channel`}
                value={draft.announceChannel}
                disabled={locked}
                error={problems.announceChannel ? t('peerbonus.slack.channel.required') : undefined}
                onChange={(channel) => edit({ type: 'channel', channel })}
              />
            </div>
          </div>

          <div className="mt-4 divide-y divide-stone-100 border-y border-stone-100">
            <SwitchRow
              label={t('peerbonus.slack.announcePosts')}
              hint={t('peerbonus.slack.announcePostsHint')}
              checked={draft.announcePosts}
              disabled={locked}
              onChange={(value) => edit({ type: 'toggle', field: 'announcePosts', value })}
            />
            <SwitchRow
              label={t('peerbonus.slack.dmRecipientOnPost')}
              hint={t('peerbonus.slack.dmRecipientOnPostHint')}
              checked={draft.dmRecipientOnPost}
              disabled={locked}
              onChange={(value) => edit({ type: 'toggle', field: 'dmRecipientOnPost', value })}
            />
            <SwitchRow
              label={t('peerbonus.slack.giveFromSlack')}
              hint={
                slack.credential.signingSecret ? (
                  t('peerbonus.slack.giveFromSlackHint')
                ) : (
                  <span className={cn(problems.giveFromSlack ? 'text-bad-ink' : undefined)}>{t('peerbonus.slack.giveFromSlackLocked')}</span>
                )
              }
              checked={draft.giveFromSlack}
              // Turning it off stays possible after the secret was removed.
              disabled={locked || (!slack.credential.signingSecret && !draft.giveFromSlack)}
              onChange={(value) => edit({ type: 'toggle', field: 'giveFromSlack', value })}
            />
          </div>

          <div className="mt-4">
            <label htmlFor={`${id}-locale`} className="text-sm font-medium text-stone-800">
              {t('peerbonus.slack.locale')}
            </label>
            <select
              id={`${id}-locale`}
              value={draft.locale}
              aria-describedby={`${id}-locale-hint`}
              onChange={(event) => edit({ type: 'locale', locale: event.target.value as SlackLocale })}
              className={cn(controlClasses, 'mt-1.5 h-10 w-full pr-8 sm:w-48 md:h-9')}
            >
              {LANGUAGES.map((language) => (
                <option key={language} value={language}>
                  {t(`peerbonus.language.${language}`)}
                </option>
              ))}
            </select>
            <p id={`${id}-locale-hint`} className="mt-1.5 text-xs text-stone-500">
              {t('peerbonus.slack.localeHint')}
            </p>
          </div>
        </fieldset>

        <div className="mt-5 space-y-3">
          {save.isError ? <SlackActionError error={save.error} className="py-2" /> : null}
          {test.isError ? <SlackActionError error={test.error} channel={savedChannel?.name} className="py-2" /> : null}
          {saved && !dirty ? (
            <Notice tone="success" className="py-2">
              {t('peerbonus.admin.program.saved')}
            </Notice>
          ) : null}
          {test.isSuccess && savedChannel ? (
            <Notice tone="success" className="py-2">
              {t('peerbonus.slack.testSent', { channel: savedChannel.name })}
            </Notice>
          ) : null}
          <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
            <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
              <Button
                icon={<Send />}
                loading={test.isPending}
                disabled={locked || dirty || savedChannel === null}
                aria-describedby={dirty && savedChannel ? `${id}-test-hint` : undefined}
                onClick={() => {
                  setSaved(false)
                  test.mutate()
                }}
              >
                {t('peerbonus.slack.test')}
              </Button>
              <span id={`${id}-test-hint`} className="text-xs text-stone-500" aria-live="polite">
                {dirty ? (savedChannel ? t('peerbonus.slack.testSaveFirst') : t('peerbonus.admin.program.unsaved')) : null}
              </span>
            </div>
            <div className="ml-auto flex gap-2">
              <Button disabled={!dirty || save.isPending} onClick={() => edit({ type: 'reset', settings: slack.settings })}>
                {t('peerbonus.admin.program.discard')}
              </Button>
              <Button type="submit" tone="primary" icon={<Save />} loading={save.isPending} disabled={!dirty || hasProblems || locked}>
                {t('peerbonus.admin.program.save')}
              </Button>
            </div>
          </div>
        </div>
      </form>
    </Card>
  )
}
