import { useEffect, useState } from 'react'

/** `value` once it stopped changing for `delay` ms (search fields ask the server after typing settles). */
export function useDebounced(value: string, delay: number): string {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])
  return settled
}
