import { Button } from '../../Button'
import { Section, SettingRow } from '../../ui/Section'
import { useRoomStore } from '../RoomContext'
import { RestartButton } from '../RestartButton'
import { Whisper } from './Whisper'

export function RoomTools() {
  const store = useRoomStore()
  const holder = store.remoteHolder.value
  const holderName = holder ? store.users.value.get(holder)?.nickname ?? 'Someone' : ''
  return <>
    <Section title="Desktop">
      <SettingRow title="Reset the remote" description={holder ? `${holderName} holds the remote. Take it away from them.` : 'Nobody holds the remote right now.'}>
        <Button icon="mouse" disabled={!holder} onClick={() => store.resetRemote()}>Reset remote</Button>
      </SettingRow>
      {store.restartAvailable.value && (
        <SettingRow title="Restart the room" description="Restarts the desktop for everyone, for when the stream or the browser is stuck.">
          <RestartButton />
        </SettingRow>
      )}
    </Section>
    <Section title="Whisper" description="A message in chat that only one person sees. It is not kept.">
      <Whisper />
    </Section>
  </>
}
