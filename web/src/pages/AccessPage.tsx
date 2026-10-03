import { useEffect, useState } from 'preact/hooks'
import { useLocation, useRoute } from 'preact-iso'
import { api } from '../api'
import { InfoScreen } from '../components/InfoScreen'
import { Button } from '../components/Button'

export function AccessPage() {
  const {
    params: { code },
  } = useRoute()
  const { route } = useLocation()
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    setError('')
    void api
      .get<{ room: string; temporary: boolean }>(
        `/api/invites/${encodeURIComponent(code)}`,
      )
      .then((invite) => {
        if (!active) return
        if (!invite.temporary) {
          setError('Invalid access link')
          return
        }
        route(
          `/room/${encodeURIComponent(invite.room)}?access=${encodeURIComponent(code)}`,
        )
      })
      .catch((e) => {
        if (active)
          setError(e instanceof Error ? e.message : 'Something went wrong.')
      })
    return () => {
      active = false
    }
  }, [code])
  return (
    <InfoScreen
      message={error ? 'Error' : 'checking Access'}
      submessage={error || 'please wait'}
    >
      <Button accent onClick={() => route('/')}>
        Home
      </Button>
    </InfoScreen>
  )
}
