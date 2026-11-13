import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { AlertTriangle, Lock, Plus, Save, Trash2, X } from 'lucide-react'
import { useEffect, useId, useReducer, useRef, useState, type FormEvent } from 'react'
import { peerbonusApi, type ProgramAdmin, type SlackAdmin, type SlackSettings } from '../../../api/peerbonus'
import { useText } from '../../../i18n'
import { Button, cn, controlClasses, Field, IconButton, Notice, TextInput } from '../../../ui'
import { slackQuery, SLACK_KEY } from '../queries'
import { Card, SwitchRow } from './parts'
import { SlackActionError } from './slack-error'
import { conversationBlocker } from './slack-state'
import {
  conversationDraftOf,
  conversationPatchOf,
  conversationReducer,
  isEverydayEmoji,
  MAX_POINT_CHOICES,
  MAX_REACTION_RULES,
  newRowId,
  normalizeSlackEmoji,
  validateConversation,
  type ConversationAction,
} from './slack-form'

/**
 * 会話の中で贈る (design reaction-give): the switch for mentions and reaction emoji, the point choices
 * the bot offers when a mention omits points, the emoji → points rules, and how to use it. Saved
 * together; locked until the workspace is connected. The switch needs the signing secret (the server
 * refuses without it) and the inbound URL (Slack has nowhere to send events otherwise); turning it
 * off stays possible. Row problems show once the field was left or a save was tried. A save replaces
 * the settings; the form restarts from what the server stored (`key` by the caller).
 */
export function ConversationSettingsCard({
  slack,
  program,
  justSaved,
  onSaved,
}: {
  slack: SlackAdmin
  program: ProgramAdmin
  justSaved: boolean
  onSaved: (settings: SlackSettings) => void
}) {
  const { t } = useText()
  const client = useQueryClient()
  const id = useId()
  const [draft, dispatch] = useReducer(conversationReducer, slack.settings, conversationDraftOf)
  const [saved, setSaved] = useState(justSaved)
  const [attempted, setAttempted] = useState(false)
  const [touched, setTouched] = useState<readonly string[]>([])
  const rootRef = useRef<HTMLFormElement>(null)
  // After add / remove, the new row (or the neighbour, or the add button) takes focus once it renders.
  const focusNext = useRef<string | null>(null)
  useEffect(() => {
    const target = focusNext.current
    if (!target) return
    focusNext.current = null
    rootRef.current?.querySelector<HTMLElement>(`[data-focus="${CSS.escape(target)}"]`)?.focus()
  })

  const locked = slack.state !== 'connected'
  const max = program.maxPointsPerPost
  const problems = validateConversation(draft, { signingSecret: slack.credential.signingSecret, maxPointsPerPost: max, valueKeys: program.values.map((value) => value.key) })
  const patch = conversationPatchOf(draft, slack.settings)
  const dirty = Object.keys(patch).length > 0
  const hasProblems = Object.keys(problems).length > 0
  const shown = (field: string) => attempted || touched.includes(field)
  const touch = (field: string) => setTouched((fields) => (fields.includes(field) ? fields : [...fields, field]))

  const save = useMutation({
    mutationFn: () => peerbonusApi.admin.slack.update(patch),
    onSuccess: async (result) => {
      onSaved(result.settings)
      client.setQueryData(slackQuery.queryKey, result)
      await client.invalidateQueries({ queryKey: SLACK_KEY })
    },
  })

  useBlocker({
    shouldBlockFn: () => dirty && !save.isPending && !window.confirm(t('peerbonus.admin.program.leave')),
    enableBeforeUnload: () => dirty,
  })

  const edit = (action: ConversationAction) => {
    dispatch(action)
    setSaved(false)
    if (save.isError) save.reset()
  }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    setAttempted(true)
    if (!dirty || hasProblems || locked) return
    save.mutate()
  }

  const choiceErrors = Object.entries(problems.choices ?? {}).filter(([rowId]) => shown(`${rowId}-points`))
  const choiceError = choiceErrors.some(([, problem]) => problem === 'range')
    ? t('peerbonus.slack.conversation.error.pointsRange', { max })
    : choiceErrors.length > 0
      ? t('peerbonus.slack.conversation.error.choiceDuplicate')
      : undefined
  const blocker = conversationBlocker(slack)

  return (
    <Card title={t('peerbonus.slack.conversation.title')} lead={t('peerbonus.slack.conversation.lead')}>
      <form ref={rootRef} noValidate onSubmit={submit}>
        {locked ? (
          <p className="mb-4 flex items-center gap-2 text-sm text-stone-600">
            <Lock aria-hidden className="size-4 shrink-0 text-stone-400" />
            {t('peerbonus.slack.settings.locked')}
          </p>
        ) : null}
        <fieldset disabled={locked} className="min-w-0">
          <div className="divide-y divide-stone-100 border-y border-stone-100">
            <SwitchRow
              label={t('peerbonus.slack.conversation.toggle')}
              hint={blocker ? <span className={cn(problems.giveInConversation ? 'text-bad-ink' : undefined)}>{t(blocker)}</span> : t('peerbonus.slack.conversation.toggleHint')}
              checked={draft.giveInConversation}
              // Turning it off stays possible after the secret or the endpoint went away.
              disabled={locked || (blocker !== null && !draft.giveInConversation)}
              onChange={(value) => edit({ type: 'toggle', value })}
            />
          </div>

          <div role="group" aria-labelledby={`${id}-choices-label`} aria-describedby={`${id}-choices-${choiceError ? 'error' : 'hint'}`} className="mt-4">
            <p id={`${id}-choices-label`} className="text-sm font-medium text-stone-800">
              {t('peerbonus.slack.conversation.choices')}
            </p>
            <ul className="mt-1.5 flex flex-wrap items-center gap-2">
              {draft.choices.map((row, index) => {
                const invalid = problems.choices?.[row.rowId] !== undefined && shown(`${row.rowId}-points`)
                return (
                  <li key={row.rowId} className="flex items-center">
                    <TextInput
                      type="number"
                      inputMode="numeric"
                      min={1}
                      max={max}
                      step={1}
                      data-focus={`${row.rowId}-points`}
                      aria-label={t('peerbonus.slack.conversation.choice', { index: index + 1 })}
                      aria-describedby={invalid ? `${id}-choices-error` : undefined}
                      value={row.points}
                      invalid={invalid}
                      className="w-20 rounded-r-none tabular-nums"
                      onChange={(event) => edit({ type: 'setChoice', rowId: row.rowId, points: event.target.value })}
                      onBlur={() => touch(`${row.rowId}-points`)}
                    />
                    <IconButton
                      icon={<X />}
                      label={t('peerbonus.slack.conversation.removeChoice', { index: index + 1 })}
                      tone="secondary"
                      disabled={draft.choices.length <= 1}
                      className="-ml-px h-10 rounded-l-none md:h-9"
                      onClick={() => {
                        edit({ type: 'removeChoice', rowId: row.rowId })
                        const neighbour = draft.choices[index + 1] ?? draft.choices[index - 1]
                        focusNext.current = neighbour ? `${neighbour.rowId}-points` : `${id}-add-choice`
                      }}
                    />
                  </li>
                )
              })}
              <li className="flex">
                <Button
                  size="sm"
                  icon={<Plus />}
                  data-focus={`${id}-add-choice`}
                  disabled={draft.choices.length >= MAX_POINT_CHOICES}
                  onClick={() => {
                    const rowId = newRowId('choice')
                    edit({ type: 'addChoice', rowId })
                    focusNext.current = `${rowId}-points`
                  }}
                >
                  {t('peerbonus.slack.conversation.addChoice')}
                </Button>
              </li>
            </ul>
            {choiceError ? (
              <p id={`${id}-choices-error`} className="mt-1.5 text-xs text-bad-ink">
                {choiceError}
              </p>
            ) : (
              <p id={`${id}-choices-hint`} className="mt-1.5 text-xs text-stone-500">
                {t('peerbonus.slack.conversation.choicesHint', { limit: MAX_POINT_CHOICES, max })}
              </p>
            )}
          </div>

          <div className="mt-5">
            <p id={`${id}-rules-label`} className="text-sm font-medium text-stone-800">
              {t('peerbonus.slack.conversation.rules')}
            </p>
            <p className="mt-0.5 text-xs text-stone-500">{t('peerbonus.slack.conversation.rulesHint')}</p>
            {draft.rules.length === 0 ? (
              <p className="mt-2 rounded-lg border border-dashed border-stone-300 px-4 py-4 text-center text-sm text-stone-500">{t('peerbonus.slack.conversation.rulesEmpty')}</p>
            ) : (
              <ol aria-labelledby={`${id}-rules-label`} className="mt-2 divide-y divide-stone-100 rounded-lg border border-line">
                {draft.rules.map((row, index) => {
                  const base = `${id}-${row.rowId}`
                  const position = index + 1
                  const rule = problems.rules?.[row.rowId] ?? {}
                  const emojiProblem = shown(`${row.rowId}-emoji`) ? rule.emoji : undefined
                  const pointsProblem = shown(`${row.rowId}-points`) ? rule.points : undefined
                  const everyday = !rule.emoji && isEverydayEmoji(row.emoji)
                  const staleValue = row.valueKey !== '' && !program.values.some((value) => value.key === row.valueKey)
                  return (
                    <li key={row.rowId} aria-label={t('peerbonus.slack.conversation.rule', { index: position })} className="flex items-start gap-2 p-3 sm:gap-3">
                      <div className="grid min-w-0 flex-1 gap-3 sm:grid-cols-[minmax(0,16rem)_6rem_minmax(0,12rem)]">
                        <Field
                          id={`${base}-emoji`}
                          label={t('peerbonus.slack.conversation.emoji')}
                          error={
                            emojiProblem === 'required'
                              ? t('peerbonus.slack.conversation.error.emojiRequired')
                              : emojiProblem === 'invalid'
                                ? t('peerbonus.slack.conversation.error.emojiInvalid')
                                : emojiProblem === 'duplicate'
                                  ? t('peerbonus.slack.conversation.error.emojiDuplicate')
                                  : undefined
                          }
                          hint={
                            everyday ? (
                              <span className="inline-flex items-start gap-1 text-busy-ink">
                                <AlertTriangle aria-hidden className="mt-px size-3.5 shrink-0" />
                                {t('peerbonus.slack.conversation.everyday', { emoji: normalizeSlackEmoji(row.emoji) })}
                              </span>
                            ) : undefined
                          }
                        >
                          <div className="relative">
                            <span aria-hidden className="pointer-events-none absolute inset-y-0 left-2.5 flex items-center font-mono text-sm text-stone-400">
                              :
                            </span>
                            <TextInput
                              id={`${base}-emoji`}
                              data-focus={`${row.rowId}-emoji`}
                              value={row.emoji}
                              invalid={Boolean(emojiProblem)}
                              placeholder="pb-10"
                              maxLength={100}
                              autoCapitalize="none"
                              autoComplete="off"
                              spellCheck={false}
                              className="px-5 font-mono"
                              onChange={(event) => edit({ type: 'setRule', rowId: row.rowId, change: { emoji: event.target.value } })}
                              onBlur={() => {
                                touch(`${row.rowId}-emoji`)
                                const normalized = normalizeSlackEmoji(row.emoji)
                                if (normalized !== row.emoji) edit({ type: 'setRule', rowId: row.rowId, change: { emoji: normalized } })
                              }}
                            />
                            <span aria-hidden className="pointer-events-none absolute inset-y-0 right-2.5 flex items-center font-mono text-sm text-stone-400">
                              :
                            </span>
                          </div>
                        </Field>
                        <Field
                          id={`${base}-points`}
                          label={t('peerbonus.slack.conversation.points')}
                          error={pointsProblem ? t('peerbonus.slack.conversation.error.pointsRange', { max }) : undefined}
                        >
                          <TextInput
                            id={`${base}-points`}
                            type="number"
                            inputMode="numeric"
                            min={1}
                            max={max}
                            step={1}
                            value={row.points}
                            invalid={Boolean(pointsProblem)}
                            className="tabular-nums"
                            onChange={(event) => edit({ type: 'setRule', rowId: row.rowId, change: { points: event.target.value } })}
                            onBlur={() => touch(`${row.rowId}-points`)}
                          />
                        </Field>
                        <Field
                          id={`${base}-value`}
                          label={t('peerbonus.slack.conversation.value')}
                          error={rule.valueKey ? t('peerbonus.slack.conversation.error.valueUnknown') : undefined}
                        >
                          <select
                            id={`${base}-value`}
                            value={row.valueKey}
                            aria-invalid={rule.valueKey ? true : undefined}
                            aria-describedby={rule.valueKey ? `${base}-value-error` : undefined}
                            onChange={(event) => edit({ type: 'setRule', rowId: row.rowId, change: { valueKey: event.target.value } })}
                            className={cn(controlClasses, 'h-10 w-full pr-8 md:h-9')}
                          >
                            <option value="">{t('peerbonus.slack.conversation.noValue')}</option>
                            {program.values.map((value) => (
                              <option key={value.key} value={value.key}>
                                {value.label}
                              </option>
                            ))}
                            {staleValue ? <option value={row.valueKey}>{t('peerbonus.slack.conversation.removedValue', { key: row.valueKey })}</option> : null}
                          </select>
                        </Field>
                      </div>
                      <IconButton
                        icon={<Trash2 />}
                        label={t('peerbonus.slack.conversation.removeRule', { index: position })}
                        className="mt-6.5 text-bad-ink hover:bg-bad-soft hover:text-bad-ink md:mt-7"
                        onClick={() => {
                          edit({ type: 'removeRule', rowId: row.rowId })
                          const neighbour = draft.rules[index + 1] ?? draft.rules[index - 1]
                          focusNext.current = neighbour ? `${neighbour.rowId}-emoji` : `${id}-add-rule`
                        }}
                      />
                    </li>
                  )
                })}
              </ol>
            )}
            <div className="mt-2 flex items-center justify-between gap-3">
              <Button
                size="sm"
                icon={<Plus />}
                data-focus={`${id}-add-rule`}
                disabled={draft.rules.length >= MAX_REACTION_RULES}
                onClick={() => {
                  const rowId = newRowId('rule')
                  edit({ type: 'addRule', rowId })
                  focusNext.current = `${rowId}-emoji`
                }}
              >
                {t('peerbonus.slack.conversation.addRule')}
              </Button>
              <span className="text-xs text-stone-500 tabular-nums" aria-live="polite">
                {t('peerbonus.slack.conversation.rulesCount', { count: draft.rules.length, max: MAX_REACTION_RULES })}
              </span>
            </div>
          </div>
        </fieldset>

        <div className="mt-5 rounded-lg bg-stone-50 px-3 py-3 sm:px-4">
          <h3 className="text-sm font-medium text-stone-800">{t('peerbonus.slack.conversation.help')}</h3>
          <ul className="mt-1.5 list-disc space-y-1 pl-5 text-sm text-stone-600 marker:text-stone-400">
            <li>
              {t('peerbonus.slack.conversation.helpMention')}{' '}
              <code className="rounded bg-white px-1.5 py-0.5 font-mono text-xs text-stone-800 ring-1 ring-stone-200 ring-inset">
                {t('peerbonus.slack.conversation.helpExample')}
              </code>
            </li>
            <li>{t('peerbonus.slack.conversation.helpAsk')}</li>
            <li>{t('peerbonus.slack.conversation.helpInvite')}</li>
            <li>{t('peerbonus.slack.conversation.helpReinstall')}</li>
          </ul>
        </div>

        <div className="mt-5 space-y-3">
          {save.isError ? <SlackActionError error={save.error} className="py-2" /> : null}
          {saved && !dirty ? (
            <Notice tone="success" className="py-2">
              {t('peerbonus.admin.program.saved')}
            </Notice>
          ) : null}
          <div className="flex flex-wrap items-center justify-end gap-x-4 gap-y-3">
            <span className="text-xs text-stone-500" aria-live="polite">
              {dirty ? t('peerbonus.admin.program.unsaved') : null}
            </span>
            <div className="flex gap-2">
              <Button
                disabled={!dirty || save.isPending}
                onClick={() => {
                  edit({ type: 'reset', settings: slack.settings })
                  setAttempted(false)
                  setTouched([])
                }}
              >
                {t('peerbonus.admin.program.discard')}
              </Button>
              <Button type="submit" tone="primary" icon={<Save />} loading={save.isPending} disabled={!dirty || locked || (attempted && hasProblems)}>
                {t('peerbonus.admin.program.save')}
              </Button>
            </div>
          </div>
        </div>
      </form>
    </Card>
  )
}
