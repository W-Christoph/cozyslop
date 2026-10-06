import { useEffect, useRef } from 'preact/hooks'
import { LocationProvider, Route, Router, useLocation } from 'preact-iso'
import { Header } from '../components/Header'
import { InfoScreen } from '../components/InfoScreen'
import { AccessPage } from '../pages/AccessPage'
import { AdminPage } from '../pages/admin/AdminPage'
import { HomePage } from '../pages/HomePage'
import { InvitePage } from '../pages/InvitePage'
import { LoginPage } from '../pages/LoginPage'
import { LicensePage } from '../pages/LicensePage'
import { NotFoundPage } from '../pages/NotFoundPage'
import { ResetPasswordPage } from '../pages/ResetPasswordPage'
import { RegisterPage } from '../pages/RegisterPage'
import { RoomRoute } from '../pages/RoomPage'
import { SettingsDialog } from '../components/settings/SettingsDialog'
import { meLoaded, pageTitle, refreshMe, refreshServerSettings, settingsOpen, type SettingsSection } from './state'
import styles from './App.module.css'

export function App() {
  useEffect(() => {
    void refreshMe().catch(() => {})
    void refreshServerSettings().catch(() => {})
  }, [])

  return (
    <LocationProvider>
      <Shell />
    </LocationProvider>
  )
}

const settingsRoutes: Record<string, SettingsSection | undefined> = { '/profile': 'account', '/settings': 'appearance' }

function Shell() {
  const { path, route } = useLocation()
  const previous = useRef(path)
  const inRoom = path.startsWith('/room/')
  useEffect(() => {
    if (!inRoom) pageTitle.value = null
  }, [inRoom])
  // /profile and /settings open the settings window over the rooms page.
  // Otherwise the window belongs to the page it was opened on.
  useEffect(() => {
    const opens = settingsRoutes[path]
    if (opens) {
      settingsOpen.value = opens
      route('/', true)
    } else if (!settingsRoutes[previous.current]) settingsOpen.value = null
    previous.current = path
  }, [path])
  if (!inRoom && !meLoaded.value) {
    return (
      <InfoScreen
        busy
        message="Connecting to CozyCast…"
        submessage="If this takes too long please refresh"
      />
    )
  }
  const routes = (
    <Router>
      <Route path="/" component={HomePage} />
      <Route path="/room/:room" component={RoomRoute} />
      <Route path="/reset/:token" component={ResetPasswordPage} />
      <Route path="/invite/:code" component={InvitePage} />
      <Route path="/access/:code" component={AccessPage} />
      <Route path="/login" component={LoginPage} />
      <Route path="/license" component={LicensePage} />
      <Route path="/register" component={RegisterPage} />
      <Route path="/profile" component={HomePage} />
      <Route path="/settings" component={HomePage} />
      <Route path="/admin/:tab?" component={AdminPage} />
      <Route default component={NotFoundPage} />
    </Router>
  )
  // The room fills the window and opens the settings itself.
  if (inRoom) return routes
  return (
    <div data-ui class={styles.app}>
      <Header />
      {routes}
      {settingsOpen.value && <SettingsDialog />}
    </div>
  )
}
