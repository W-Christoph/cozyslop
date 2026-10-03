import { render } from 'preact'
import { RoomPage } from './pages/RoomPage'
import './styles/tokens.css'
import './styles/base.css'

// Prototype routing: /room/<name>, defaulting to "default".
const room = location.pathname.match(/^\/room\/([^/]+)/)?.[1] ?? 'default'
const name = new URLSearchParams(location.search).get('name') ?? 'Anonymous'

render(<RoomPage room={decodeURIComponent(room)} name={name} />, document.getElementById('app')!)
