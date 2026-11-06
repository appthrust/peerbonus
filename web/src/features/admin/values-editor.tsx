import { ArrowDown, ArrowUp, ChevronRight, Plus, Trash2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useText } from '../../i18n'
import { Button, ConfirmDialog, Field, IconButton, TextInput } from '../../ui'
import { emptyValue, MAX_VALUES, moveRow, setKey, setLabel, type ValueDraft, type ValueProblems } from './program-form'

/**
 * The program's values (0–20 rows): a label and an optional description, added, removed (with a
 * confirmation once the row has content) and reordered in place. The key posts store is generated
 * from the label and shown muted ("key: thanks"); 詳細 opens it for editing, and opens by itself
 * when the key has a problem. The order here is the order participants see in the compose form.
 */
export function ValuesEditor({
  id,
  rows,
  problems,
  onChange,
}: {
  id: string
  rows: ValueDraft[]
  /** Per `rowId`; only passed once errors should show. */
  problems: Record<string, ValueProblems>
  onChange: (rows: ValueDraft[]) => void
}) {
  const { t } = useText()
  const [keyOpen, setKeyOpen] = useState<readonly string[]>([])
  const [removing, setRemoving] = useState<number | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  // After add / move / remove, the row (or button) that moved takes focus once the new rows render, so keyboard users do not lose their place.
  const focusNext = useRef<string | null>(null)

  useEffect(() => {
    const target = focusNext.current
    if (!target) return
    focusNext.current = null
    rootRef.current?.querySelector<HTMLElement>(`[data-focus="${CSS.escape(target)}"]`)?.focus()
  })

  const add = () => {
    const row = emptyValue()
    onChange([...rows, row])
    focusNext.current = `${row.rowId}-label`
  }
  const move = (index: number, delta: -1 | 1) => {
    onChange(moveRow(rows, index, delta))
    const target = index + delta
    // At the new edge the pressed button disables itself; focus the opposite one.
    const edge = delta === -1 ? target === 0 : target === rows.length - 1
    focusNext.current = `${rows[index]!.rowId}-${edge ? (delta === -1 ? 'down' : 'up') : delta === -1 ? 'up' : 'down'}`
  }
  const remove = (index: number) => {
    onChange(rows.filter((_, i) => i !== index))
    const neighbour = rows[index + 1] ?? rows[index - 1]
    focusNext.current = neighbour ? `${neighbour.rowId}-label` : `${id}-add`
  }
  // A saved value or one with text asks first (posts keep its key; the label is lost); a blank new row just goes.
  const askRemove = (index: number) => {
    const row = rows[index]!
    if (row.autoKey && !row.label.trim() && !row.description.trim()) remove(index)
    else setRemoving(index)
  }
  const removingRow = removing === null ? undefined : rows[removing]

  return (
    <div ref={rootRef}>
      {rows.length === 0 ? (
        <p className="rounded-lg border border-dashed border-stone-300 bg-white px-4 py-6 text-center text-sm text-stone-500">{t('peerbonus.admin.program.valuesEmpty')}</p>
      ) : (
        <ol className="space-y-3">
          {rows.map((row, index) => {
            const rowProblems = problems[row.rowId] ?? {}
            const base = `${id}-${row.rowId}`
            const position = index + 1
            const showKey = Boolean(rowProblems.key) || keyOpen.includes(row.rowId)
            return (
              <li key={row.rowId} aria-label={t('peerbonus.admin.program.valueRow', { index: position })} className="rounded-lg border border-line bg-white p-3 sm:p-4">
                <div className="flex items-start gap-2 sm:gap-3">
                  <span aria-hidden className="mt-8 w-5 shrink-0 text-center text-xs font-medium text-stone-400 tabular-nums md:mt-7">
                    {position}
                  </span>
                  <div className="grid min-w-0 flex-1 gap-3 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]">
                    <Field
                      id={`${base}-label`}
                      label={t('peerbonus.admin.program.valueLabel')}
                      error={rowProblems.label ? t('peerbonus.admin.program.error.valueLabel') : undefined}
                    >
                      <TextInput
                        id={`${base}-label`}
                        data-focus={`${row.rowId}-label`}
                        value={row.label}
                        invalid={Boolean(rowProblems.label)}
                        onChange={(event) => onChange(setLabel(rows, row.rowId, event.target.value))}
                      />
                    </Field>
                    <Field id={`${base}-description`} label={t('peerbonus.admin.program.valueDescription')}>
                      <TextInput id={`${base}-description`} value={row.description} onChange={(event) => onChange(rows.map((item) => (item.rowId === row.rowId ? { ...item, description: event.target.value } : item)))} />
                    </Field>
                    <div className="sm:col-span-2">
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-stone-500">
                        <span className="min-w-0 truncate font-mono">{t('peerbonus.admin.program.keyShown', { key: row.key.trim() || '—' })}</span>
                        <button
                          type="button"
                          aria-expanded={showKey}
                          aria-controls={`${base}-key-panel`}
                          disabled={Boolean(rowProblems.key)}
                          onClick={() => setKeyOpen((open) => (open.includes(row.rowId) ? open.filter((rowId) => rowId !== row.rowId) : [...open, row.rowId]))}
                          className="inline-flex min-h-10 items-center gap-0.5 rounded font-medium text-stone-600 hover:text-stone-900 disabled:cursor-default md:min-h-0"
                        >
                          <ChevronRight aria-hidden className={showKey ? 'size-3.5 rotate-90 transition-transform' : 'size-3.5 transition-transform'} />
                          {t('peerbonus.admin.program.keyDetails')}
                        </button>
                      </div>
                      {showKey ? (
                        <div id={`${base}-key-panel`} className="mt-2 max-w-sm">
                          <Field
                            id={`${base}-key`}
                            label={t('peerbonus.admin.program.valueKey')}
                            hint={row.autoKey ? t('peerbonus.admin.program.valueKeyHint') : t('peerbonus.admin.program.valueKeySavedHint')}
                            error={
                              rowProblems.key === 'duplicate'
                                ? t('peerbonus.admin.program.error.valueKeyDuplicate')
                                : rowProblems.key
                                  ? t('peerbonus.admin.program.error.valueKey')
                                  : undefined
                            }
                          >
                            <TextInput
                              id={`${base}-key`}
                              value={row.key}
                              invalid={Boolean(rowProblems.key)}
                              maxLength={32}
                              autoCapitalize="none"
                              autoComplete="off"
                              spellCheck={false}
                              className="font-mono"
                              onChange={(event) => onChange(setKey(rows, row.rowId, event.target.value))}
                            />
                          </Field>
                        </div>
                      ) : null}
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-col items-center gap-0.5 sm:mt-6 sm:flex-row">
                    <IconButton
                      data-focus={`${row.rowId}-up`}
                      icon={<ArrowUp />}
                      label={t('peerbonus.admin.program.moveUp', { index: position })}
                      disabled={index === 0}
                      onClick={() => move(index, -1)}
                    />
                    <IconButton
                      data-focus={`${row.rowId}-down`}
                      icon={<ArrowDown />}
                      label={t('peerbonus.admin.program.moveDown', { index: position })}
                      disabled={index === rows.length - 1}
                      onClick={() => move(index, 1)}
                    />
                    <IconButton
                      icon={<Trash2 />}
                      label={t('peerbonus.admin.program.removeValue', { index: position })}
                      className="text-bad-ink hover:bg-bad-soft hover:text-bad-ink"
                      onClick={() => askRemove(index)}
                    />
                  </div>
                </div>
              </li>
            )
          })}
        </ol>
      )}
      <div className="mt-3 flex items-center justify-between gap-3">
        <Button id={`${id}-add`} data-focus={`${id}-add`} size="sm" icon={<Plus />} disabled={rows.length >= MAX_VALUES} onClick={add}>
          {t('peerbonus.admin.program.addValue')}
        </Button>
        <span className="text-xs text-stone-500 tabular-nums" aria-live="polite">
          {t('peerbonus.admin.program.valuesCount', { count: rows.length, max: MAX_VALUES })}
        </span>
      </div>
      <ConfirmDialog
        open={removingRow !== undefined}
        onOpenChange={(open) => {
          if (!open) setRemoving(null)
        }}
        title={t('peerbonus.admin.program.removeTitle', { label: removingRow?.label.trim() || t('peerbonus.admin.program.valueRow', { index: (removing ?? 0) + 1 }) })}
        body={t('peerbonus.admin.program.removeBody', { key: removingRow?.key.trim() || '—' })}
        confirmLabel={t('peerbonus.admin.program.removeConfirm')}
        tone="danger"
        onConfirm={() => {
          if (removing !== null) remove(removing)
          setRemoving(null)
        }}
      />
    </div>
  )
}
