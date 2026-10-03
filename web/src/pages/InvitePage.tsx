import { useEffect, useState } from 'preact/hooks'
import { useLocation, useRoute } from 'preact-iso'
import { api } from '../api'
import { me, pendingInvite } from '../app/state'
import { InfoScreen } from '../components/InfoScreen'
import { Button } from '../components/Button'

export function InvitePage() {
  const {
    params: { code },
  } = useRoute()
  const { route } = useLocation()
  const user = me.value
  const [state, setState] = useState({
    message: 'checking invite',
    submessage: 'please wait',
    login: false,
    room: '',
  })
  useEffect(() => {
    let active = true
    setState({
      message: 'checking invite',
      submessage: 'please wait',
      login: false,
      room: '',
    })
    void (async () => {
      try {
        const invite = await api.get<{ room: string; temporary: boolean }>(
          `/api/invites/${encodeURIComponent(code)}`,
        )
        if (!active) return
        if (invite.temporary)
          throw new Error('Invalid invite. This is a temporary access link.')
        if (user) {
          await api.post(`/api/invites/${encodeURIComponent(code)}/redeem`)
          if (active) {
            pendingInvite.clear()
            setState({
              message: 'Success',
              submessage: `You have been invited to ${invite.room}`,
              login: false,
              room: invite.room,
            })
          }
        } else {
          pendingInvite.set(code)
          setState({
            message: 'Not logged in',
            submessage: 'Please log in to use an invite',
            login: true,
            room: '',
          })
        }
      } catch (e) {
        if (active)
          setState({
            message: 'Error',
            submessage:
              e instanceof Error ? e.message : 'Something went wrong.',
            login: false,
            room: '',
          })
      }
    })()
    return () => {
      active = false
    }
  }, [code, user?.username])
  return (
    <InfoScreen message={state.message} submessage={state.submessage}>
      {state.login ? (
        <>
          <Button accent onClick={() => route('/login')}>
            Login
          </Button>
          <Button accent onClick={() => route('/register')}>
            Register
          </Button>
        </>
      ) : (
        <>
          {state.room && (
            <a href={`/room/${encodeURIComponent(state.room)}`}>{state.room}</a>
          )}
          <Button accent onClick={() => route('/')}>
            Home
          </Button>
        </>
      )}
    </InfoScreen>
  )
}
