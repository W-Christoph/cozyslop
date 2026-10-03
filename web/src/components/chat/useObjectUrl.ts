import { useLayoutEffect, useState } from 'preact/hooks'

export function useObjectUrl(blob: Blob): string {
  const [url, setUrl] = useState('')
  useLayoutEffect(() => {
    const source = URL.createObjectURL(blob)
    setUrl(source)
    return () => URL.revokeObjectURL(source)
  }, [blob])
  return url
}
