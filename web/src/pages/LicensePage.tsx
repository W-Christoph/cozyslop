import { serverSettings } from '../app/state'
import { PageLayout } from '../components/PageLayout'
import styles from './LicensePage.module.css'

// Linked from the room footer. The AGPL requires offering the source code to
// everyone who uses the server over the network; this page does that.
export function LicensePage() {
  const source = serverSettings.value.sourceUrl
  return (
    <PageLayout narrow title="License" subtitle="CozyCast is free software.">
      <div class={styles.notice}>
        <p>
          CozyCast — movie night over the internet
          <br />
          Copyright (C) 2024 Vorlent, W-Christoph
        </p>
        <p>
          This program is free software: you can redistribute it and/or modify it under the terms of the{' '}
          <a href="https://www.gnu.org/licenses/agpl-3.0.html" target="_blank" rel="noopener noreferrer">
            GNU Affero General Public License
          </a>{' '}
          as published by the Free Software Foundation, either version 3 of the License, or (at your option) any
          later version. It is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
          the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
        </p>
        <p>
          The source code of this server is available at{' '}
          <a href={source} target="_blank" rel="noopener noreferrer">
            {source}
          </a>
          .
        </p>
      </div>
    </PageLayout>
  )
}
