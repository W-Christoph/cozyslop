import { useEffect } from 'preact/hooks'
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
import { ProfilePage } from '../pages/ProfilePage'
import { RegisterPage } from '../pages/RegisterPage'
import { RoomRoute } from '../pages/RoomPage'
import { SettingsPage } from '../pages/SettingsPage'
import { meLoaded, pageTitle, refreshMe, refreshServerSettings } from './state'

export function App() {
  useEffect(() => {
    void refreshMe().catch(() => {})
    void refreshServerSettings().catch(() => {})
  }, [])

  if (!meLoaded.value) {
    return (
      <InfoScreen
        message="Connecting to CozyCast..."
        submessage="If this takes too long please refresh"
      />
    )
  }

  return (
    <LocationProvider>
      <Shell />
    </LocationProvider>
  )
}

function Shell() {
  const { path } = useLocation()
  const inRoom = path.startsWith('/room/')
  useEffect(() => {
    if (!inRoom) pageTitle.value = null
  }, [inRoom])
  return (
    <>
      {!inRoom && <Header />}
      <Router>
        <Route path="/" component={HomePage} />
        <Route path="/room/:room" component={RoomRoute} />
        <Route path="/invite/:code" component={InvitePage} />
        <Route path="/access/:code" component={AccessPage} />
        <Route path="/login" component={LoginPage} />
        <Route path="/license" component={LicensePage} />
        <Route path="/register" component={RegisterPage} />
        <Route path="/profile" component={ProfilePage} />
        <Route path="/settings" component={SettingsPage} />
        <Route path="/admin/:tab?" component={AdminPage} />
        <Route default component={NotFoundPage} />
      </Router>
    </>
  )
}
