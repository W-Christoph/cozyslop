import { useEffect, useState } from 'preact/hooks'
import { preferences, updatePreferences } from '../app/state'
import { PageLayout } from '../components/PageLayout'
import styles from './SettingsPage.module.css'

export function SettingsPage() {
  const [message, setMessage] = useState(''),
    [revision, setRevision] = useState(0)
  useEffect(() => {
    if (!message) return
    const timeout = window.setTimeout(() => setMessage(''), 2500)
    return () => clearTimeout(timeout)
  }, [revision])
  function change(field: 'manualLoadMedia' | 'audioOnly', value: boolean) {
    updatePreferences({ [field]: value })
    setMessage('Media settings updated!')
    setRevision((n) => n + 1)
  }
  return (
    <PageLayout panel={false}>
      <div class={styles.panel}>
        <div class={styles.title}>MEDIA PREFERENCES</div>
        <div class={styles.settings}>
          <div class={styles.manual}>
            <label>
              <input
                type="checkbox"
                checked={preferences.value.manualLoadMedia}
                onChange={(e) =>
                  change('manualLoadMedia', e.currentTarget.checked)
                }
              />
              Manually Load Images and Videos
            </label>
            <p>
              Note: This is only useful if your internet connection is very
              slow.
            </p>
          </div>
          <div class={styles.audio}>
            <label>
              <input
                type="checkbox"
                checked={preferences.value.audioOnly}
                onChange={(e) => change('audioOnly', e.currentTarget.checked)}
              />
              Stream Music Only
            </label>
            <p>Disables video feeds to save bandwidth.</p>
          </div>
        </div>
        <p class={styles.confirmation} role="status">
          {message}
        </p>
      </div>
    </PageLayout>
  )
}
