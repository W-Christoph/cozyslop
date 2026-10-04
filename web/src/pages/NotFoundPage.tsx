import { useLocation } from 'preact-iso'
import { InfoScreen } from '../components/InfoScreen'
import { Button } from '../components/Button'

export function NotFoundPage() {
  const { route } = useLocation()
  return (
    <InfoScreen message="Page not found" submessage="This page does not exist or has moved." icon="search">
      <Button accent size="lg" onClick={() => route('/')}>
        Back to rooms
      </Button>
    </InfoScreen>
  )
}
