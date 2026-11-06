// The program settings form as plain data: the editable draft built from
// `GET /admin/program`, the same checks the server applies (spec §3.7,
// api.md §4 "PATCH program"), and the minimal PATCH body for what changed.
import type { ProgramAdmin, ProgramPatch, Value } from '../../api/peerbonus'

export const MAX_WEEKLY_ALLOWANCE = 10_000
export const MAX_VALUES = 20
const VALUE_KEY = /^[a-z0-9-]{1,32}$/
const WHOLE = /^\d+$/
const DECIMAL = /^\d+(\.\d+)?$/
const CURRENCY = /^[A-Z]{3}$/

/**
 * One values row while editing; `rowId` keeps React keys and errors stable across reorders.
 * `autoKey`: the key still follows the label (a row added in this edit whose key nobody typed);
 * a saved row never changes its key by itself, since posts store it.
 */
export type ValueDraft = { rowId: string; key: string; label: string; description: string; autoKey: boolean }

/** Numbers stay text while typing, so a half-typed "0." is not lost. */
export type ProgramDraft = {
  timeZone: string
  weeklyAllowance: string
  maxPointsPerPost: string
  pointUnitValue: string
  currency: string
  values: ValueDraft[]
}

export type ValueProblems = { key?: 'invalid' | 'duplicate'; label?: 'required' }

export type ProgramProblems = {
  timeZone?: 'invalid'
  weeklyAllowance?: 'range'
  /** `noAllowance`: the weekly points are 0, so no maximum fits. */
  maxPointsPerPost?: 'range' | 'noAllowance'
  pointUnitValue?: 'invalid'
  currency?: 'invalid'
  /** Keyed by `ValueDraft.rowId`. */
  values: Record<string, ValueProblems>
}

let rowSeq = 0
/** A fresh local row id (never sent to the server). */
export const newRowId = () => `value-${++rowSeq}`

export const emptyValue = (): ValueDraft => ({ rowId: newRowId(), key: '', label: '', description: '', autoKey: true })

export function draftOf(program: ProgramAdmin): ProgramDraft {
  return {
    timeZone: program.timeZone,
    weeklyAllowance: String(program.weeklyAllowance),
    maxPointsPerPost: String(program.maxPointsPerPost),
    pointUnitValue: program.pointUnitValue,
    currency: program.currency,
    values: program.values.map((value) => ({ rowId: newRowId(), key: value.key, label: value.label, description: value.description ?? '', autoKey: false })),
  }
}

/** Whether the runtime knows the IANA zone (the server re-checks with Go's tz database). */
export function isKnownTimeZone(zone: string): boolean {
  if (!zone || zone !== zone.trim() || zone === 'Local') return false
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: zone })
    return true
  } catch {
    return false
  }
}

const wholeIn = (text: string, min: number, max: number) => {
  const trimmed = text.trim()
  if (!WHOLE.test(trimmed)) return undefined
  const value = Number(trimmed)
  return value >= min && value <= max ? value : undefined
}

/** The submitted values: trimmed, an empty description is null. */
export function valuesOf(draft: ProgramDraft): Value[] {
  return draft.values.map((value) => ({
    key: value.key.trim(),
    label: value.label.trim(),
    description: value.description.trim() === '' ? null : value.description.trim(),
  }))
}

/** Every rule the server enforces on PATCH /admin/program. `timeZoneLocked` skips the zone (it cannot be edited then). */
export function validateProgram(draft: ProgramDraft, { timeZoneLocked = false, isTimeZone = isKnownTimeZone } = {}): ProgramProblems {
  const problems: ProgramProblems = { values: {} }
  if (!timeZoneLocked && !isTimeZone(draft.timeZone)) problems.timeZone = 'invalid'
  const allowance = wholeIn(draft.weeklyAllowance, 0, MAX_WEEKLY_ALLOWANCE)
  if (allowance === undefined) problems.weeklyAllowance = 'range'
  if (allowance === 0) problems.maxPointsPerPost = 'noAllowance'
  else if (wholeIn(draft.maxPointsPerPost, 1, allowance ?? MAX_WEEKLY_ALLOWANCE) === undefined) problems.maxPointsPerPost = 'range'
  if (!DECIMAL.test(draft.pointUnitValue.trim())) problems.pointUnitValue = 'invalid'
  if (!CURRENCY.test(draft.currency.trim())) problems.currency = 'invalid'
  const seen = new Set<string>()
  for (const row of draft.values) {
    const rowProblems: ValueProblems = {}
    const key = row.key.trim()
    if (!VALUE_KEY.test(key)) rowProblems.key = 'invalid'
    else if (seen.has(key)) rowProblems.key = 'duplicate'
    seen.add(key)
    if (row.label.trim() === '') rowProblems.label = 'required'
    if (rowProblems.key || rowProblems.label) problems.values[row.rowId] = rowProblems
  }
  return problems
}

export function hasProblems(problems: ProgramProblems): boolean {
  return Boolean(
    problems.timeZone || problems.weeklyAllowance || problems.maxPointsPerPost || problems.pointUnitValue || problems.currency || Object.keys(problems.values).length > 0
  )
}

const sameValues = (a: Value[], b: Value[]) =>
  a.length === b.length && a.every((value, index) => value.key === b[index]!.key && value.label === b[index]!.label && (value.description ?? null) === (b[index]!.description ?? null))

/**
 * The PATCH body: only the fields that differ from the saved program; `values` is sent whole
 * (the server replaces the list). An empty object means nothing to save. Call after validation passed.
 */
export function patchOf(draft: ProgramDraft, saved: ProgramAdmin): ProgramPatch {
  const patch: ProgramPatch = {}
  const timeZone = draft.timeZone.trim()
  if (timeZone !== saved.timeZone) patch.timeZone = timeZone
  const allowance = Number(draft.weeklyAllowance.trim())
  if (allowance !== saved.weeklyAllowance) patch.weeklyAllowance = allowance
  const max = Number(draft.maxPointsPerPost.trim())
  if (max !== saved.maxPointsPerPost) patch.maxPointsPerPost = max
  const unit = draft.pointUnitValue.trim()
  if (unit !== saved.pointUnitValue) patch.pointUnitValue = unit
  const currency = draft.currency.trim()
  if (currency !== saved.currency) patch.currency = currency
  const values = valuesOf(draft)
  if (!sameValues(values, saved.values)) patch.values = values
  return patch
}

/** Unsaved edits, compared as submitted (so trailing spaces alone are not "changed"). */
export function isDirty(draft: ProgramDraft, saved: ProgramAdmin): boolean {
  const patch = patchOf(draft, saved)
  // A number field holding text that is not a number yet is still an edit.
  return Object.keys(patch).length > 0 || draft.weeklyAllowance.trim() !== String(saved.weeklyAllowance) || draft.maxPointsPerPost.trim() !== String(saved.maxPointsPerPost)
}

/** Moves the row at `index` by `delta` (−1 up, +1 down); out of range is a no-op. */
export function moveRow<T>(rows: readonly T[], index: number, delta: -1 | 1): T[] {
  const target = index + delta
  if (index < 0 || index >= rows.length || target < 0 || target >= rows.length) return [...rows]
  const next = [...rows]
  ;[next[index], next[target]] = [next[target]!, next[index]!]
  return next
}

// Hepburn romaji for hiragana (katakana is folded onto hiragana first). Two-kana
// entries (きゃ) win over one-kana ones; っ doubles the next consonant; ー is dropped.
const KANA: Record<string, string> = Object.fromEntries(
  (
    'あa いi うu えe おo かka きki くku けke こko さsa しshi すsu せse そso たta ちchi つtsu てte とto なna にni ぬnu ねne のno ' +
    'はha ひhi ふfu へhe ほho まma みmi むmu めme もmo やya ゆyu よyo らra りri るru れre ろro わwa をo んn ' +
    'がga ぎgi ぐgu げge ごgo ざza じji ずzu ぜze ぞzo だda ぢji づzu でde どdo ばba びbi ぶbu べbe ぼbo ぱpa ぴpi ぷpu ぺpe ぽpo ' +
    'ぁa ぃi ぅu ぇe ぉo ゃya ゅyu ょyo ゎwa ゔvu ' +
    'きゃkya きゅkyu きょkyo しゃsha しゅshu しょsho ちゃcha ちゅchu ちょcho にゃnya にゅnyu にょnyo ひゃhya ひゅhyu ひょhyo ' +
    'みゃmya みゅmyu みょmyo りゃrya りゅryu りょryo ぎゃgya ぎゅgyu ぎょgyo じゃja じゅju じょjo びゃbya びゅbyu びょbyo ぴゃpya ぴゅpyu ぴょpyo ' +
    'てぃti でぃdi ふぁfa ふぃfi ふぇfe ふぉfo うぃwi うぇwe うぉwo しぇshe ちぇche じぇje'
  )
    .split(' ')
    .map((entry) => [entry.replace(/[a-z]+$/, ''), entry.replace(/^[^a-z]+/, '')])
)

/**
 * The value key a label suggests: Latin letters and digits as they are, kana romanized, everything
 * else (kanji, symbols, spaces) a hyphen; collapsed, at most 32 characters. Empty when nothing
 * romanizable remains ("感謝" → "").
 */
export function romanizeKey(label: string): string {
  const text = label
    .normalize('NFKC')
    .toLowerCase()
    .replace(/[\u30a1-\u30f6]/g, (char) => String.fromCharCode(char.charCodeAt(0) - 0x60))
  let out = ''
  let double = false
  for (let index = 0; index < text.length; index++) {
    const char = text[index]!
    if (char === 'ー') continue
    if (char === 'っ') {
      double = true
      continue
    }
    const pair = KANA[text.slice(index, index + 2)]
    const romaji = pair ?? KANA[char]
    if (pair) index++
    if (romaji) {
      out += (double && /^[^aeiou]/.test(romaji) ? romaji[0] : '') + romaji
    } else {
      out += /[a-z0-9]/.test(char) ? char : '-'
    }
    double = false
  }
  return out.replace(/-+/g, '-').slice(0, 32).replace(/^-|-$/g, '')
}

/**
 * A key for `label` that no other row uses: the romanized label (with "-2", "-3"… on a clash), or
 * "value-N" (the smallest free N) when the label has nothing to romanize.
 */
export function keyFor(label: string, taken: ReadonlySet<string>): string {
  const base = romanizeKey(label)
  if (base) {
    if (!taken.has(base)) return base
    for (let n = 2; ; n++) {
      const suffix = `-${n}`
      const candidate = `${base.slice(0, 32 - suffix.length).replace(/-$/, '')}${suffix}`
      if (!taken.has(candidate)) return candidate
    }
  }
  for (let n = 1; ; n++) if (!taken.has(`value-${n}`)) return `value-${n}`
}

/** Sets a row's label; a row whose key still follows its label gets the matching key. */
export function setLabel(rows: readonly ValueDraft[], rowId: string, label: string): ValueDraft[] {
  const taken = new Set(rows.filter((row) => row.rowId !== rowId).map((row) => row.key.trim()))
  return rows.map((row) => {
    if (row.rowId !== rowId) return row
    return { ...row, label, key: row.autoKey ? (label.trim() ? keyFor(label, taken) : '') : row.key }
  })
}

/** Sets a row's key by hand; from then on the label no longer changes it. */
export function setKey(rows: readonly ValueDraft[], rowId: string, key: string): ValueDraft[] {
  return rows.map((row) => (row.rowId === rowId ? { ...row, key, autoKey: false } : row))
}
