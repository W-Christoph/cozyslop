package hub

import (
	"cozycast/internal/rights"
	"cozycast/internal/store"
)

// Room WebSocket protocol. Every message is a JSON object with a "type".
// Mirrored in web/src/room/protocol.ts; keep both in sync.

// ---- server -> browser ----------------------------------------------------

// User is a person in the room as everyone else sees them.
type User struct {
	Key       string `json:"key"`      // identity key, "u:<id>" or "a:<anon id>"
	Username  string `json:"username"` // account name; "" for anonymous users
	Nickname  string `json:"nickname"`
	NameColor string `json:"nameColor"`
	AvatarURL string `json:"avatarUrl"`
	Anonymous bool   `json:"anonymous"`
	Admin     bool   `json:"admin"`
	Active    bool   `json:"active"` // tab visible
	Muted     bool   `json:"muted"`  // not listening to the stream
	JoinedAt  int64  `json:"joinedAt"`
	LastSeen  int64  `json:"lastSeen"` // unix ms; when they were last active
}

// ChatMessage is a chat line as sent to browsers.
type ChatMessage struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"` // identity key
	Nickname  string `json:"nickname"`
	NameColor string `json:"nameColor"`
	AvatarURL string `json:"avatarUrl"`
	Anonymous bool   `json:"anonymous"`
	Type      string `json:"type"` // text | image | video | whisper
	Body      string `json:"body"`
	MediaURL  string `json:"mediaUrl,omitempty"`
	Edited    bool   `json:"edited"`
	Time      int64  `json:"time"` // unix ms
}

type welcomeMsg struct {
	Type     string             `json:"type"` // "welcome"
	ClientID string             `json:"clientId"`
	Self     User               `json:"self"`
	Rights   rights.Rights      `json:"rights"`
	Settings store.RoomSettings `json:"settings"`
	Users    []User             `json:"users"`
	History  []ChatMessage      `json:"history"` // oldest first; replaces what the browser had
	Remote   *string            `json:"remote"`  // identity key of the remote holder
	Restart  bool               `json:"restart"` // the room can be restarted (admins, trusted)
	// Title of the window in front on the desktop; "" if unknown.
	WindowTitle string `json:"windowTitle"`
}

type windowTitleMsg struct {
	Type  string `json:"type"` // "window_title"
	Title string `json:"title"`
}

type nekoMsg struct {
	Type  string `json:"type"` // "neko"
	Token string `json:"token"`
	Path  string `json:"path"`
}

type nekoUnavailableMsg struct {
	Type    string `json:"type"` // "neko_unavailable"
	Message string `json:"message"`
}

type userMsg struct {
	Type string `json:"type"` // "user_joined" | "user_updated"
	User User   `json:"user"`
}

type userLeftMsg struct {
	Type string `json:"type"` // "user_left"
	Key  string `json:"key"`
}

type chatMsg struct {
	Type    string      `json:"type"` // "chat"
	Message ChatMessage `json:"message"`
}

type chatEditedMsg struct {
	Type string `json:"type"` // "chat_edited"
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

type chatDeletedMsg struct {
	Type string `json:"type"` // "chat_deleted"
	ID   int64  `json:"id"`
}

type typingMsg struct {
	Type   string `json:"type"` // "typing"
	Key    string `json:"key"`
	Typing bool   `json:"typing"`
}

type rightsMsg struct {
	Type   string        `json:"type"` // "rights"
	Rights rights.Rights `json:"rights"`
}

type settingsMsg struct {
	Type     string             `json:"type"` // "room_settings"
	Settings store.RoomSettings `json:"settings"`
}

type remoteMsg struct {
	Type   string  `json:"type"`   // "remote"
	Holder *string `json:"holder"` // identity key, nil = nobody
}

// Answer to file_delete and file_play, to the tab that asked.
type fileResultMsg struct {
	Type   string `json:"type"`   // "file_result"
	Action string `json:"action"` // "delete" | "play"
	Name   string `json:"name"`
	Error  string `json:"error"` // "" if it worked
}

// The room's desktop is restarting; streams come back by themselves.
type restartingMsg struct {
	Type string `json:"type"` // "restarting"
	By   string `json:"by"`   // nickname of who restarted it
}

// Sent right before the server closes the socket on purpose.
type kickedMsg struct {
	Type        string `json:"type"`   // "kicked"
	Reason      string `json:"reason"` // "banned" | "account" | "verified" | "invite" | "kicked" | "deleted" | "not_found" | "session"
	BannedUntil *int64 `json:"bannedUntil,omitempty"`
}

type errorMsg struct {
	Type    string `json:"type"` // "error"
	Message string `json:"message"`
}

// ---- browser -> server ----------------------------------------------------

// ClientMsg is any message a browser may send. Fields not used by a type are
// ignored.
type ClientMsg struct {
	Type   string `json:"type"`
	Body   string `json:"body"`   // chat_send, chat_edit, whisper
	ID     int64  `json:"id"`     // chat_edit, chat_delete
	Typing bool   `json:"typing"` // typing
	Active bool   `json:"active"` // activity
	Muted  bool   `json:"muted"`  // muted
	To     string `json:"to"`     // whisper: identity key
	Name   string `json:"name"`   // file_delete, file_play: a file in the desktop's Downloads
}
