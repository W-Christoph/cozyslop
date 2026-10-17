import { Fragment } from 'preact'
import { useLayoutEffect, useRef } from 'preact/hooks'
import { applyNameColors } from '../chat/nameColor'
import { me, preferences, resolveTheme } from '../../app/state'
import { ChatAvatar } from '../chat/ChatAvatar'
import chat from '../chat/MessageGroup.module.css'
import list from '../chat/MessageList.module.css'
import styles from './ChatPreview.module.css'

// The room's chat with the current preferences, drawn with the chat's own
// styles so that it cannot drift from the real thing. With sized, the column
// is as wide as the chat width of the preferences makes it, and the height is
// fixed so that changing the look does not move what is around it.
export function ChatPreview({ nickname, color, avatarUrl, sized = false }: { nickname?: string; color?: string; avatarUrl?: string; sized?: boolean }) {
  const preview = useRef<HTMLDivElement>(null)
  const { chatStyle, chatAvatars, chatScale, chatWidth, showLeaveJoinMsg, theme } = preferences.value
  const user = me.value
  useLayoutEffect(() => applyNameColors(preview.current), [nickname, color, user?.nickname, user?.nameColor, chatStyle, resolveTheme(theme)])
  const avatar = chatAvatars && chatStyle !== 'compact'
  const groups = [
    { name: 'Mr.Cozy', color: '#7dd3fc', url: '/png/default_avatar_on_alpha.png', time: '8:57 PM', lines: ['Feeling cozy tonight?'] },
    {
      name: nickname ?? user?.nickname ?? 'You', color: color ?? user?.nameColor ?? '#f90',
      url: avatarUrl ?? (user ? user.avatarUrl || '/png/default_avatar.png' : undefined),
      time: '8:58 PM', lines: ['This is how your messages look.', 'A second one right after it.'],
    },
  ]
  return (
    <div ref={preview} class={`${styles.preview} ${sized ? styles.sized : ''}`} aria-label="Chat preview" role="img">
      <div class={styles.chat} data-chat-style={chatStyle} style={{ fontSize: `${(16 * chatScale) / 100}px`, '--chat-width': chatWidth / 100 }}>
        <div class={styles.messages}>
          {groups.map((group, index) => <Fragment key={group.name}>
            {(chatStyle === 'compact' ? group.lines.map((line) => [line]) : [group.lines]).map((lines, lineIndex) =>
              <div key={lineIndex} class={`${chat.message} ${avatar ? chat.withAvatar : ''}`}>
                {avatar && <ChatAvatar class={chat.avatarSlot} nickname={group.name} color={group.color} url={group.url} />}
                <div data-chat-name class={chat.username} style={{ '--name-colour': group.color }}>
                  {group.name}
                  <span class={chat.timestamp}>{group.time}</span>
                </div>
                {lines.map((line, index) => <div key={line} class={`${chat.subMessage} ${index === lines.length - 1 ? chat.last : ''}`}>{line}</div>)}
              </div>
            )}
            {index === 0 && showLeaveJoinMsg && <div class={list.temporary}><span>Pixel joined</span></div>}
          </Fragment>)}
        </div>
      </div>
    </div>
  )
}
