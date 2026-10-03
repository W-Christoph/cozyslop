import { useLocation } from 'preact-iso'
import { InfoScreen } from '../components/InfoScreen'
import { Button } from '../components/Button'

export function NotFoundPage() {
  const { route } = useLocation()
  return (
    <InfoScreen message="Page not found">
      <Button accent onClick={() => route('/')}>
        Home
      </Button>
    </InfoScreen>
  )
}
