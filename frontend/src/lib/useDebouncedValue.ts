import { useEffect, useState } from 'react'

export function useDebouncedValue<T>(value: T, delayMs = 250): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const handle = window.setTimeout(() => {
      setDebounced(value)
    }, delayMs)
    return () => window.clearTimeout(handle)
  }, [delayMs, value])

  return debounced
}
