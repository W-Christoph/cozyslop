package hub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"cozycast/internal/store"
)

const (
	maxChatRunes     = 4096
	maxAnonChatRunes = 250
	typingThrottle   = time.Second
)

// Handle processes one message from a tab.
func (r *Room) Handle(ctx context.Context, c *Client, msg ClientMsg) {
	r.mu.Lock()
	_, present := r.clients[c.ID]
	key := c.m.key
	r.mu.Unlock()
	if !present {
		return // kicked; its socket is closing
	}
	// Typing, presence and edits reach the whole room too, so everything a
	// person sends counts here; new chat messages have a tighter limit.
	if !r.inbound.Allow(key) {
		c.send(errorMsg{Type: "error", Message: "You are sending messages too fast."})
		return
	}

	var err error
	switch msg.Type {
	case "chat_send":
		err = r.sendChat(ctx, c, msg.Body)
	case "chat_edit":
		err = r.editChat(ctx, c, msg.ID, msg.Body)
	case "chat_delete":
		err = r.deleteChat(ctx, c, msg.ID)
	case "typing":
		r.typing(c, msg.Typing)
	case "activity":
		r.setPresence(c, func() { c.active = msg.Active })
	case "muted":
		r.setPresence(c, func() { c.muted = msg.Muted })
	case "whisper":
		err = r.whisper(c, msg.To, msg.Body)
	case "remote_reset":
		if r.isAdmin(c) {
			err = r.ResetRemote(ctx)
		} else {
			err = ErrNotAllowed
		}
	case "neko_token":
		r.sendNekoToken(ctx, c)
	case "restart":
		err = r.Restart(c)
	default:
		c.send(errorMsg{Type: "error", Message: "Unknown message type."})
		return
	}

	var userErr *userError
	switch {
	case err == nil:
	case errors.As(err, &userErr):
		c.send(errorMsg{Type: "error", Message: userErr.msg})
	case errors.Is(err, ErrRateLimited):
		c.send(errorMsg{Type: "error", Message: "You are sending messages too fast."})
	case errors.Is(err, ErrNotAllowed):
		c.send(errorMsg{Type: "error", Message: "You are not allowed to do that."})
	default:
		r.log.Error("handle message", "type", msg.Type, "client", c.ID, "err", err)
		c.send(errorMsg{Type: "error", Message: "Something went wrong."})
	}
}

// userError is a failure explained to the user as-is.
type userError struct{ msg string }

func (e *userError) Error() string { return e.msg }

// SendNekoToken issues the tab a neko token (on join, and whenever its neko
// connection was rejected, e.g. after the room container restarted).
func (r *Room) SendNekoToken(ctx context.Context, c *Client) { r.sendNekoToken(ctx, c) }

func (r *Room) sendNekoToken(ctx context.Context, c *Client) {
	token, err := r.NekoToken(ctx, c)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, ErrNotPresent) {
			r.log.Error("issue neko token", "client", c.ID, "err", err)
			c.send(errorMsg{Type: "error", Message: "The room's desktop is not reachable right now."})
		}
		return
	}
	c.send(nekoMsg{Type: "neko", Token: token, Path: r.NekoPath})
}

// ---- chat -----------------------------------------------------------------

func (r *Room) checkChatBody(c *Client, body string) (string, error) {
	body = strings.Trim(body, "\r\n")
	if strings.TrimSpace(body) == "" {
		return "", &userError{"Message is empty."}
	}
	r.mu.Lock()
	anonymous := c.m.user == nil
	r.mu.Unlock()
	limit := maxChatRunes
	if anonymous {
		limit = maxAnonChatRunes
	}
	if utf8.RuneCountInString(body) > limit {
		return "", &userError{"Message is too long."}
	}
	return body, nil
}

func (r *Room) sendChat(ctx context.Context, c *Client, body string) error {
	body, err := r.checkChatBody(c, body)
	if err != nil {
		return err
	}
	return r.post(ctx, c.m.key, "text", body, "")
}

// CanPostMedia checks presence, image rights and the chat rate limit before
// an upload is read, consuming one chat token.
// PostMedia checks them again once the upload is ready.
func (r *Room) CanPostMedia(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.members[key]
	if m == nil {
		return ErrNotPresent
	}
	if !m.rights.Image {
		return ErrNotAllowed
	}
	return r.chatRateLocked(m)
}

// PostMedia adds an uploaded image or video to the chat on behalf of the
// person with the given identity key. They must be in the room with the
// image right. Call CanPostMedia before reading the upload.
func (r *Room) PostMedia(ctx context.Context, key, typ, file string) error {
	r.mu.Lock()
	m := r.members[key]
	allowed := m != nil && m.rights.Image
	r.mu.Unlock()
	if m == nil {
		return ErrNotPresent
	}
	if !allowed {
		return ErrNotAllowed
	}
	return r.post(ctx, key, typ, "", file)
}

func (r *Room) post(ctx context.Context, key, typ, body, media string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.members[key]
	if m == nil {
		return ErrNotPresent
	}

	if typ == "text" {
		if err := r.chatRateLocked(m); err != nil {
			return err
		}
	}

	u := r.userLocked(m)
	msg := &store.ChatMessage{
		Room:      r.Name,
		Identity:  m.key,
		Nickname:  u.Nickname,
		NameColor: u.NameColor,
		Anonymous: u.Anonymous,
		Type:      typ,
		Body:      body,
		Media:     media,
	}
	if m.user != nil {
		msg.UserID = &m.user.ID
	}
	// Insert and broadcast under the lock so every tab sees messages in id
	// order and joins cannot miss one (see Join).
	if err := r.hub.store.InsertChat(ctx, msg); err != nil {
		return err
	}
	r.broadcastLocked(chatMsg{Type: "chat", Message: r.toChat([]store.ChatMessage{*msg})[0]}, nil)
	return nil
}

func (r *Room) chatRateLocked(m *member) error {
	limiter := r.chatUser
	if m.user == nil {
		limiter = r.chatAnon
	}
	if !limiter.Allow(m.key) {
		return ErrRateLimited
	}
	return nil
}

func (r *Room) editChat(ctx context.Context, c *Client, id int64, body string) error {
	body, err := r.checkChatBody(c, body)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Only the author edits, and only text.
	err = r.hub.store.EditChat(ctx, r.Name, id, c.m.key, body)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotAllowed
	}
	if err != nil {
		return err
	}
	r.broadcastLocked(chatEditedMsg{Type: "chat_edited", ID: id, Body: body}, nil)
	return nil
}

func (r *Room) deleteChat(ctx context.Context, c *Client, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	author := c.m.key
	if c.m.rights.Admin {
		author = "" // admins delete anyone's messages
	}
	media, err := r.hub.store.DeleteChat(ctx, r.Name, id, author)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotAllowed
	}
	if err != nil {
		return err
	}
	r.hub.removeMedia([]string{media})
	r.broadcastLocked(chatDeletedMsg{Type: "chat_deleted", ID: id}, nil)
	return nil
}

// whisper sends a private message from an admin to one person. It is not
// stored.
func (r *Room) whisper(c *Client, to, body string) error {
	if !r.isAdmin(c) {
		return ErrNotAllowed
	}
	body, err := r.checkChatBody(c, body)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	target := r.members[to]
	if target == nil {
		return &userError{"That user is not in the room anymore."}
	}
	msg := chatMsg{Type: "chat", Message: ChatMessage{
		ID:        -r.whisperID.Add(1), // negative: never collides with stored ids
		Author:    c.m.key,
		Nickname:  "Mod Whisper",
		NameColor: "#fff",
		Type:      "whisper",
		Body:      body,
		Time:      time.Now().UnixMilli(),
	}}
	for _, tc := range target.clients {
		tc.send(msg)
	}
	if target != c.m {
		for _, sc := range c.m.clients {
			sc.send(msg)
		}
	}
	return nil
}

func (r *Room) toChat(msgs []store.ChatMessage) []ChatMessage {
	out := make([]ChatMessage, len(msgs))
	for i, m := range msgs {
		out[i] = ChatMessage{
			ID:        m.ID,
			Author:    m.Identity,
			Nickname:  m.Nickname,
			NameColor: m.NameColor,
			Anonymous: m.Anonymous,
			Type:      m.Type,
			Body:      m.Body,
			Edited:    m.Edited,
			Time:      m.CreatedAtMs,
		}
		if m.Media != "" {
			out[i].MediaURL = "/media/chat/" + m.Media
		}
	}
	return out
}

// ---- presence -------------------------------------------------------------

func (r *Room) typing(c *Client, typing bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if typing && now.Sub(c.typing) < typingThrottle {
		return
	}
	c.typing = now
	r.broadcastLocked(typingMsg{Type: "typing", Key: c.m.key, Typing: typing}, c.m)
}

// setPresence applies a change to one tab's activity or mute state and
// announces the person's combined state if it changed.
func (r *Room) setPresence(c *Client, change func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.userLocked(c.m)
	change()
	after := r.userLocked(c.m)
	if before.Active && !after.Active {
		c.m.lastSeen = time.Now().UnixMilli()
		after.LastSeen = c.m.lastSeen
	}
	if after != before {
		r.broadcastLocked(userMsg{Type: "user_updated", User: after}, nil)
	}
}

func (r *Room) isAdmin(c *Client) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return c.m.rights.Admin
}

// RestartCooldown is how often trusted users may restart a room; admins may
// restart it any time.
const RestartCooldown = time.Hour

// Restart restarts the room's container on behalf of a tab, if container
// control is enabled and the tab's person may do it.
func (r *Room) Restart(c *Client) error {
	if r.restart == nil {
		return &userError{"Restarting is not enabled on this server."}
	}
	r.mu.Lock()
	rt := c.m.rights
	if !rt.Admin && !rt.Trusted {
		r.mu.Unlock()
		return ErrNotAllowed
	}
	if wait := time.Until(r.lastRestart.Add(RestartCooldown)); !rt.Admin && wait > 0 {
		r.mu.Unlock()
		return &userError{fmt.Sprintf("The room was restarted recently. Try again in %d minutes.", int(wait.Minutes())+1)}
	}
	r.lastRestart = time.Now()
	r.broadcastLocked(restartingMsg{Type: "restarting", By: r.userLocked(c.m).Nickname}, nil)
	r.mu.Unlock()

	r.log.Info("restarting room", "by", c.m.key)
	// The restart outlives the request: the requester's socket may close
	// while the desktop goes down.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := r.restart(ctx); err != nil {
			r.log.Error("restart room", "err", err)
		}
	}()
	return nil
}
