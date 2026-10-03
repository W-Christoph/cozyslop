import { loggedIn, logout, me, serverSettings } from '../app/state'

// Placeholder navigation; ported from the old CozyCast Header next.
export function Header() {
  return (
    <header>
      <a href="/">Rooms</a> <a href="/settings">Settings</a>{' '}
      {loggedIn.value ? (
        <>
          <a href="/profile">{me.value?.nickname}</a> {me.value?.admin && <a href="/admin">Admin</a>}{' '}
          <button onClick={() => void logout()}>Logout</button>
        </>
      ) : (
        <>
          <a href="/login">Login</a> {serverSettings.value.registration === 'open' && <a href="/register">Register</a>}
        </>
      )}
    </header>
  )
}
