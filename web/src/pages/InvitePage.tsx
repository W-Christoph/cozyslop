import { useEffect, useState } from 'preact/hooks'
import { useLocation, useRoute } from 'preact-iso'
import { api } from '../api'
import { me, pendingInvite } from '../app/state'
import { InfoScreen } from '../components/InfoScreen'
import { Button, ButtonLink } from '../components/Button'

export function InvitePage() {
  const {
    params: { code },
  } = useRoute()
  const { route } = useLocation()
  const user = me.value
  const [state, setState] = useState({
    message: 'Checking invite',
    submessage: 'Please wait',
    login: false,
    room: '',
  })
  useEffect(() => {
    let active = true
    setState({
      message: 'Checking invite',
      submessage: 'Please wait',
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
              message: "You're invited",
              submessage: `You now have access to ${invite.room}.`,
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
            message: 'Invite not usable',
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
  const checking = state.message === 'Checking invite'
  return (
    <InfoScreen
      message={state.message}
      submessage={state.submessage}
      busy={checking}
      icon={state.room ? 'check' : state.login ? 'ticket' : 'alert'}
    >
      {state.login ? (
        <>
          <Button accent size="lg" onClick={() => route('/login')}>
            Log in
          </Button>
          <Button size="lg" onClick={() => route('/register')}>
            Sign up
          </Button>
        </>
      ) : (
        <>
          {state.room && (
            <ButtonLink accent size="lg" href={`/room/${encodeURIComponent(state.room)}`}>
              Join {state.room}
            </ButtonLink>
          )}
          {!checking && (
            <Button size="lg" onClick={() => route('/')}>
              All rooms
            </Button>
          )}
        </>
      )}
    </InfoScreen>
  )
}
