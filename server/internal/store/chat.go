package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Chat retention, as in CozyCast: the last hour, at most this many messages
// per room.
const (
	ChatRetention   = time.Hour
	ChatMaxMessages = 1000
)

type ChatMessage struct {
	ID          int64
	Room        string
	Identity    string // "u:<id>" or "a:<anon id>"
	UserID      *int64
	Nickname    string // snapshot at posting time
	NameColor   string
	Anonymous   bool
	Type        string // text | image | video
	Body        string
	Media       string // file name in the chat media dir
	Edited      bool
	CreatedAtMs int64
}

const chatColumns = "id, room, identity, user_id, nickname, name_color, anonymous, type, body, media, edited, created_at_ms"

func scanChat(row interface{ Scan(...any) error }) (ChatMessage, error) {
	var m ChatMessage
	err := row.Scan(&m.ID, &m.Room, &m.Identity, &m.UserID, &m.Nickname, &m.NameColor,
		&m.Anonymous, &m.Type, &m.Body, &m.Media, &m.Edited, &m.CreatedAtMs)
	return m, err
}

// InsertChat stores m, filling in ID and CreatedAtMs.
func (s *Store) InsertChat(ctx context.Context, m *ChatMessage) error {
	m.CreatedAtMs = s.now().UnixMilli()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_messages (room, identity, user_id, nickname, name_color, anonymous, type, body, media, edited, created_at_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		m.Room, m.Identity, m.UserID, m.Nickname, m.NameColor, m.Anonymous, m.Type, m.Body, m.Media, m.CreatedAtMs)
	if err != nil {
		return err
	}
	m.ID, err = res.LastInsertId()
	return err
}

// ChatHistory returns the room's retained messages, oldest first.
func (s *Store) ChatHistory(ctx context.Context, room string) ([]ChatMessage, error) {
	cutoff := s.now().Add(-ChatRetention).UnixMilli()
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+chatColumns+` FROM (
		   SELECT * FROM chat_messages WHERE room = ? AND created_at_ms > ? ORDER BY id DESC LIMIT ?
		 ) ORDER BY id`, room, cutoff, ChatMaxMessages)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessage
	for rows.Next() {
		m, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// EditChat replaces the body of a text message written by author.
// ErrNotFound if there is no such message.
func (s *Store) EditChat(ctx context.Context, room string, id int64, author, body string) error {
	return s.execOne(ctx,
		"UPDATE chat_messages SET body = ?, edited = 1 WHERE id = ? AND room = ? AND identity = ? AND type = 'text'",
		body, id, room, author)
}

// DeleteChat removes a message and returns its media file name ("" if none).
// With author == "" any message in the room may be deleted (admins).
func (s *Store) DeleteChat(ctx context.Context, room string, id int64, author string) (string, error) {
	var media string
	err := s.db.QueryRowContext(ctx,
		`DELETE FROM chat_messages WHERE id = ? AND room = ? AND (? = '' OR identity = ?) RETURNING media`,
		id, room, author, author).Scan(&media)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return media, err
}

// PruneChat deletes messages past retention in every room and returns the
// media files that belonged to them.
func (s *Store) PruneChat(ctx context.Context) ([]string, error) {
	cutoff := s.now().Add(-ChatRetention).UnixMilli()
	rows, err := s.db.QueryContext(ctx,
		`DELETE FROM chat_messages
		 WHERE created_at_ms <= ?
		    OR id IN (SELECT id FROM (
		         SELECT id, row_number() OVER (PARTITION BY room ORDER BY id DESC) AS n FROM chat_messages
		       ) WHERE n > ?)
		 RETURNING media`, cutoff, ChatMaxMessages)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var media []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		if m != "" {
			media = append(media, m)
		}
	}
	return media, rows.Err()
}
