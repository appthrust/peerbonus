import type { Value } from '../../api/peerbonus'
import { paths } from '../../routes/paths'
import { useSession } from '../../shell/use-session'
import { Chip, valueHue } from '../../ui'

/** The label of a value key; a key the Program no longer defines reads "#key" (principle 4). */
export function valueLabel(values: readonly Value[], key: string): string {
  return values.find((value) => value.key === key)?.label ?? `#${key}`
}

/** A value chip in the value's colour, linking to the timeline narrowed to that value. */
export function ValueTag({ valueKey, size = 'sm' }: { valueKey: string; size?: 'sm' | 'md' }) {
  const { me } = useSession()
  const values = me.program.values
  return (
    <Chip href={paths.timeline({ value: valueKey })} hue={valueHue(values, valueKey)} size={size} title={values.find((value) => value.key === valueKey)?.description ?? undefined}>
      {valueLabel(values, valueKey)}
    </Chip>
  )
}
