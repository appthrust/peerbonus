import { describe, expect, test } from 'bun:test'
import type { ProgramAdmin } from '../../api/peerbonus'
import { draftOf, emptyValue, hasProblems, isDirty, isKnownTimeZone, keyFor, moveRow, patchOf, romanizeKey, setKey, setLabel, validateProgram, type ProgramDraft } from './program-form'

const program: ProgramAdmin = {
  timeZone: 'Asia/Tokyo',
  timeZoneLocked: false,
  weeklyAllowance: 400,
  maxPointsPerPost: 120,
  clapCost: 2,
  pointUnitValue: '1',
  currency: 'JPY',
  values: [
    { key: 'challenge', label: '挑戦', description: '前例のないことに踏み出す' },
    { key: 'teamwork', label: 'チームワーク', description: null },
  ],
  updatedAt: '2026-10-01T00:00:00.000Z',
}

const edit = (change: Partial<ProgramDraft>): ProgramDraft => ({ ...draftOf(program), ...change })

describe('program form validation', () => {
  test('the stored program is valid and unchanged', () => {
    const draft = draftOf(program)
    expect(draft).toMatchObject({ weeklyAllowance: '400', maxPointsPerPost: '120', pointUnitValue: '1', currency: 'JPY' })
    expect(draft.values.map((value) => value.description)).toEqual(['前例のないことに踏み出す', ''])
    expect(hasProblems(validateProgram(draft))).toBe(false)
    expect(isDirty(draft, program)).toBe(false)
    expect(patchOf(draft, program)).toEqual({})
  })

  test('weekly allowance is a whole number 0..10000', () => {
    for (const bad of ['', '-1', '10001', '1.5', 'abc', '1e3']) expect(validateProgram(edit({ weeklyAllowance: bad })).weeklyAllowance, bad).toBe('range')
    for (const good of ['0', '10000', ' 400 ']) expect(validateProgram(edit({ weeklyAllowance: good, maxPointsPerPost: '1' })).weeklyAllowance, good).toBeUndefined()
  })

  test('max points per post is 1..weeklyAllowance; a zero allowance leaves no valid max', () => {
    expect(validateProgram(edit({ maxPointsPerPost: '0' })).maxPointsPerPost).toBe('range')
    expect(validateProgram(edit({ maxPointsPerPost: '401' })).maxPointsPerPost).toBe('range')
    expect(validateProgram(edit({ maxPointsPerPost: '400' })).maxPointsPerPost).toBeUndefined()
    expect(validateProgram(edit({ weeklyAllowance: '100' })).maxPointsPerPost).toBe('range')
    expect(validateProgram(edit({ weeklyAllowance: '0' })).maxPointsPerPost).toBe('noAllowance')
  })

  test('unit value is a nonnegative decimal and currency an uppercase ISO 4217 code', () => {
    for (const bad of ['', '-1', '1.', '.5', '1,5', 'x']) expect(validateProgram(edit({ pointUnitValue: bad })).pointUnitValue, bad).toBe('invalid')
    for (const good of ['0', '1', '0.5', '12.3456']) expect(validateProgram(edit({ pointUnitValue: good })).pointUnitValue, good).toBeUndefined()
    for (const bad of ['', 'jpy', 'JP', 'JPYY', '円']) expect(validateProgram(edit({ currency: bad })).currency, bad).toBe('invalid')
    expect(validateProgram(edit({ currency: 'USD' })).currency).toBeUndefined()
  })

  test('the time zone must be an IANA zone unless it is locked', () => {
    expect(isKnownTimeZone('Asia/Tokyo')).toBe(true)
    expect(isKnownTimeZone('Mars/Olympus')).toBe(false)
    expect(isKnownTimeZone('Local')).toBe(false)
    expect(isKnownTimeZone(' Asia/Tokyo')).toBe(false)
    expect(validateProgram(edit({ timeZone: 'Mars/Olympus' })).timeZone).toBe('invalid')
    expect(validateProgram(edit({ timeZone: 'Mars/Olympus' }), { timeZoneLocked: true }).timeZone).toBeUndefined()
  })

  test('values: key pattern, unique keys and a label', () => {
    const rows = [
      { ...emptyValue(), key: 'Bad Key', label: 'x' },
      { ...emptyValue(), key: 'ok', label: ' ' },
      { ...emptyValue(), key: 'ok', label: 'Again' },
      { ...emptyValue(), key: 'a'.repeat(33), label: 'Long' },
      { ...emptyValue(), key: 'fine-1', label: 'Fine' },
    ]
    const problems = validateProgram(edit({ values: rows })).values
    expect(problems[rows[0]!.rowId]).toEqual({ key: 'invalid' })
    expect(problems[rows[1]!.rowId]).toEqual({ label: 'required' })
    expect(problems[rows[2]!.rowId]).toEqual({ key: 'duplicate' })
    expect(problems[rows[3]!.rowId]).toEqual({ key: 'invalid' })
    expect(problems[rows[4]!.rowId]).toBeUndefined()
  })
})

describe('program form patch', () => {
  test('only changed fields are sent; values go whole, trimmed, an empty description as null', () => {
    const draft = draftOf(program)
    draft.weeklyAllowance = '300'
    draft.currency = 'USD'
    draft.values = [...draft.values, { ...emptyValue(), key: ' speed ', label: ' スピード ', description: '  ' }]
    expect(isDirty(draft, program)).toBe(true)
    expect(patchOf(draft, program)).toEqual({
      weeklyAllowance: 300,
      currency: 'USD',
      values: [
        { key: 'challenge', label: '挑戦', description: '前例のないことに踏み出す' },
        { key: 'teamwork', label: 'チームワーク', description: null },
        { key: 'speed', label: 'スピード', description: null },
      ],
    })
  })

  test('reordering values is a change; trailing spaces alone are not', () => {
    const draft = draftOf(program)
    draft.values = moveRow(draft.values, 1, -1)
    expect(patchOf(draft, program).values?.map((value) => value.key)).toEqual(['teamwork', 'challenge'])
    expect(isDirty({ ...draftOf(program), currency: 'JPY ', pointUnitValue: ' 1' }, program)).toBe(false)
  })

  test('half-typed numbers count as unsaved edits', () => {
    expect(isDirty(edit({ weeklyAllowance: '' }), program)).toBe(true)
  })

  test('moveRow swaps neighbours and ignores moves past either end', () => {
    expect(moveRow(['a', 'b', 'c'], 0, 1)).toEqual(['b', 'a', 'c'])
    expect(moveRow(['a', 'b', 'c'], 2, -1)).toEqual(['a', 'c', 'b'])
    expect(moveRow(['a', 'b', 'c'], 0, -1)).toEqual(['a', 'b', 'c'])
    expect(moveRow(['a', 'b', 'c'], 2, 1)).toEqual(['a', 'b', 'c'])
  })
})

describe('value keys from labels', () => {
  test('Latin labels become slugs; kana is romanized; kanji has nothing to romanize', () => {
    expect(romanizeKey('Customer First!')).toBe('customer-first')
    expect(romanizeKey('  Ｔｅａｍ　Ｗｏｒｋ ')).toBe('team-work')
    expect(romanizeKey('ありがとう')).toBe('arigatou')
    expect(romanizeKey('チャレンジ')).toBe('charenji')
    expect(romanizeKey('チームワーク')).toBe('chimuwaku')
    expect(romanizeKey('きっかけ')).toBe('kikkake')
    expect(romanizeKey('感謝')).toBe('')
    expect(romanizeKey('挑戦 Challenge')).toBe('challenge')
    expect(romanizeKey('x'.repeat(40))).toHaveLength(32)
  })

  test('a clash gets a suffix; nothing to romanize is the smallest free value-N', () => {
    expect(keyFor('Thanks', new Set())).toBe('thanks')
    expect(keyFor('Thanks', new Set(['thanks', 'thanks-2']))).toBe('thanks-3')
    expect(keyFor('感謝', new Set())).toBe('value-1')
    expect(keyFor('挑戦', new Set(['value-1', 'value-3']))).toBe('value-2')
    expect(keyFor('a'.repeat(32), new Set(['a'.repeat(32)]))).toBe(`${'a'.repeat(30)}-2`)
  })

  test('a new row follows its label until its key is typed; a saved row never changes its key', () => {
    const saved = draftOf(program).values
    const added = emptyValue()
    let rows = [...saved, added]
    rows = setLabel(rows, added.rowId, 'Challenge')
    // "challenge" is taken by the saved row.
    expect(rows[2]!.key).toBe('challenge-2')
    rows = setLabel(rows, added.rowId, '感謝')
    expect(rows[2]!.key).toBe('value-1')
    rows = setLabel(rows, added.rowId, '')
    expect(rows[2]!.key).toBe('')
    rows = setKey(rows, added.rowId, 'gratitude')
    rows = setLabel(rows, added.rowId, 'ありがとう')
    expect(rows[2]).toMatchObject({ key: 'gratitude', label: 'ありがとう', autoKey: false })
    rows = setLabel(rows, saved[0]!.rowId, '新しい挑戦')
    expect(rows[0]).toMatchObject({ key: 'challenge', label: '新しい挑戦' })
    expect(hasProblems(validateProgram(edit({ values: rows })))).toBe(false)
  })
})
