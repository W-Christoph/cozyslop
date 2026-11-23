// Package diag keeps what the server needs for troubleshooting, for a few
// days: its log, slow and failed requests, calls to AniList and the room
// helpers, health samples and stall reports. It is its own database
// (diagnostics.db) beside the server's data, so writing it never waits for
// theirs, it is not part of exports, and it may be deleted at any time.
//
// Nothing in it says who: no IP addresses, accounts, names, identity keys,
// file names, page addresses or search text. The log handler (Handler)
// enforces that for everything logged, here and on stderr.
package diag

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// Kinds of entries.
const (
	KindEvent    = "event"    // a log record
	KindRequest  = "request"  // a slow or failed request
	KindOutbound = "outbound" // a call to AniList or a room's helper
	KindSample   = "sample"   // a health sample
	KindStall    = "stall"    // the server stopped answering for a while
	KindBoot     = "boot"
	KindShutdown = "shutdown"
)

// How long entries are kept, and at most how many.
var (
	Retention       = 7 * 24 * time.Hour
	SampleRetention = 3 * 24 * time.Hour
	MaxEntries      = 200_000
)

// Entry is one thing that happened.
type Entry struct {
	ID    int64          `json:"id"`
	Time  time.Time      `json:"-"`
	Kind  string         `json:"kind"`
	Level slog.Level     `json:"-"`
	Room  string         `json:"room,omitempty"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

func (e Entry) MarshalJSON() ([]byte, error) {
	type entry Entry
	return json.Marshal(struct {
		entry
		Time  int64  `json:"time"` // unix ms
		Level string `json:"level"`
	}{entry(e), e.Time.UnixMilli(), e.Level.String()})
}

const schema = `
CREATE TABLE entries (
	id    INTEGER PRIMARY KEY,
	ts    INTEGER NOT NULL, -- unix ms
	kind  TEXT NOT NULL,
	level INTEGER NOT NULL,
	room  TEXT NOT NULL DEFAULT '',
	msg   TEXT NOT NULL,
	attrs TEXT NOT NULL DEFAULT ''
);
CREATE INDEX entries_ts ON entries (ts);
CREATE INDEX entries_kind_ts ON entries (kind, ts);
PRAGMA user_version = 1;
`

// Store is the diagnostics database. A nil *Store takes entries and keeps
// none, so callers need not check whether diagnostics are on.
type Store struct {
	db      *sql.DB
	now     func() time.Time
	entries chan Entry
	dropped atomic.Int64
	syncs   chan chan struct{}
	sumMu   sync.Mutex
	sums    map[sumKey]*sum // requests summarized per minute (Transport)
	dumps   string          // the stall reports' folder; "" = none
	stop    chan struct{}
	done    chan struct{}
	close   sync.Once
}

// Open opens the database at path, creating it if needed. One that cannot
// be read or is from another version is deleted and started anew: nothing
// in it is worth an error. Use ":memory:" for tests.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := open(ctx, path)
	if err != nil && path != ":memory:" {
		for _, p := range []string{path, path + "-wal", path + "-shm"} {
			if e := os.Remove(p); e != nil && !errors.Is(e, os.ErrNotExist) {
				return nil, errors.Join(err, e)
			}
		}
		db, err = open(ctx, path)
	}
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, now: time.Now, entries: make(chan Entry, 4096),
		syncs: make(chan chan struct{}), stop: make(chan struct{}), done: make(chan struct{})}
	if path != ":memory:" {
		s.dumps = DumpDir(path)
	}
	go s.write()
	return s, nil
}

func open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_txlock=immediate"
	if path != ":memory:" {
		dsn += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: one writer, and in-memory databases are per
	// connection.
	db.SetMaxOpenConns(1)
	var version int
	err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	switch {
	case err != nil:
	case version == 0:
		_, err = db.ExecContext(ctx, schema)
	case version != 1:
		err = fmt.Errorf("diagnostics: unknown version %d", version)
	default:
		_, err = db.ExecContext(ctx, "SELECT count(*) FROM entries")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Close writes what is waiting and closes the database.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.close.Do(func() { close(s.stop) })
	<-s.done
	return s.db.Close()
}

// Add keeps e. It never waits: when the writer is behind, e is dropped and
// counted (Dropped).
func (s *Store) Add(e Entry) {
	if s == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = s.now()
	}
	select {
	case s.entries <- e:
	default:
		s.dropped.Add(1)
	}
}

// Sync writes the entries added so far.
func (s *Store) Sync() {
	if s == nil {
		return
	}
	done := make(chan struct{})
	select {
	case s.syncs <- done:
		<-done
	case <-s.done:
	}
}

// Dropped is how many entries were not kept since the start.
func (s *Store) Dropped() int64 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}

// write stores entries in batches, and removes old ones every hour.
func (s *Store) write() {
	defer close(s.done)
	flush := time.NewTicker(time.Second)
	defer flush.Stop()
	sweep := time.NewTicker(time.Hour)
	defer sweep.Stop()
	minute := time.NewTicker(time.Minute)
	defer minute.Stop()
	s.sweep()
	var batch []Entry
	drain := func() {
		for {
			select {
			case e := <-s.entries:
				batch = append(batch, e)
			default:
				return
			}
		}
	}
	save := func() {
		if len(batch) > 0 {
			if err := s.insert(batch); err != nil {
				s.dropped.Add(int64(len(batch)))
			}
			batch = batch[:0]
		}
	}
	for {
		select {
		case e := <-s.entries:
			if batch = append(batch, e); len(batch) >= 200 {
				save()
			}
		case <-flush.C:
			save()
		case <-minute.C:
			batch = append(batch, s.summaries()...)
			save()
		case <-sweep.C:
			save()
			s.sweep()
		case done := <-s.syncs:
			drain()
			save()
			close(done)
		case <-s.stop:
			drain()
			batch = append(batch, s.summaries()...)
			save()
			return
		}
	}
}

func (s *Store) insert(batch []Entry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO entries (ts, kind, level, room, msg, attrs) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range batch {
		attrs := ""
		if len(e.Attrs) > 0 {
			b, err := json.Marshal(e.Attrs)
			if err != nil {
				b, _ = json.Marshal(map[string]string{"error": "attributes not stored: " + err.Error()})
			}
			attrs = string(b)
		}
		if _, err := stmt.ExecContext(ctx, e.Time.UnixMilli(), e.Kind, int(e.Level), e.Room, e.Msg, attrs); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// sweep removes entries older than their retention, and the oldest beyond
// MaxEntries.
func (s *Store) sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	now := s.now()
	_, _ = s.db.ExecContext(ctx, "DELETE FROM entries WHERE ts < ? OR (kind = ? AND ts < ?)",
		now.Add(-Retention).UnixMilli(), KindSample, now.Add(-SampleRetention).UnixMilli())
	_, _ = s.db.ExecContext(ctx, "DELETE FROM entries WHERE id <= (SELECT id FROM entries ORDER BY id DESC LIMIT 1 OFFSET ?)", MaxEntries)
	_, _ = s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	s.removeDumps(now.Add(-Retention))
}

// Filter selects entries. Zero fields select everything.
type Filter struct {
	Kinds    []string
	MinLevel *slog.Level
	Room     string
	Since    time.Time
	Until    time.Time
	Before   int64 // entries with smaller ids only: the next page
	Text     string
	Limit    int // default 200, at most 1000
}

// Entries returns the entries f selects, newest first.
func (s *Store) Entries(ctx context.Context, f Filter) ([]Entry, error) {
	if s == nil {
		return []Entry{}, nil
	}
	var where []string
	var args []any
	if len(f.Kinds) > 0 {
		where = append(where, "kind IN (?"+strings.Repeat(", ?", len(f.Kinds)-1)+")")
		for _, k := range f.Kinds {
			args = append(args, k)
		}
	}
	if f.MinLevel != nil {
		where, args = append(where, "level >= ?"), append(args, int(*f.MinLevel))
	}
	if f.Room != "" {
		where, args = append(where, "room = ?"), append(args, f.Room)
	}
	if !f.Since.IsZero() {
		where, args = append(where, "ts >= ?"), append(args, f.Since.UnixMilli())
	}
	if !f.Until.IsZero() {
		where, args = append(where, "ts < ?"), append(args, f.Until.UnixMilli())
	}
	if f.Before > 0 {
		where, args = append(where, "id < ?"), append(args, f.Before)
	}
	if f.Text != "" {
		where = append(where, `(instr(lower(msg), lower(?)) > 0 OR instr(lower(attrs), lower(?)) > 0)`)
		args = append(args, f.Text, f.Text)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	limit = min(limit, 1000)
	q := "SELECT id, ts, kind, level, room, msg, attrs FROM entries"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY id DESC LIMIT ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Entry{}
	for rows.Next() {
		var e Entry
		var ts int64
		var level int
		var attrs string
		if err := rows.Scan(&e.ID, &ts, &e.Kind, &level, &e.Room, &e.Msg, &attrs); err != nil {
			return nil, err
		}
		e.Time, e.Level = time.UnixMilli(ts), slog.Level(level)
		if attrs != "" {
			_ = json.Unmarshal([]byte(attrs), &e.Attrs)
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// Last returns the newest entry, if there is one.
func (s *Store) Last(ctx context.Context) (Entry, bool, error) {
	list, err := s.Entries(ctx, Filter{Limit: 1})
	if err != nil || len(list) == 0 {
		return Entry{}, false, err
	}
	return list[0], true, nil
}

// Clear removes every entry.
func (s *Store) Clear(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM entries"); err != nil {
		return err
	}
	s.removeDumps(time.Time{})
	_, err := s.db.ExecContext(ctx, "VACUUM")
	return err
}

// Size is the database's size in bytes.
func (s *Store) Size(ctx context.Context) (int64, error) {
	if s == nil {
		return 0, nil
	}
	var pages, size int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return 0, err
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&size); err != nil {
		return 0, err
	}
	return pages * size, nil
}

// Boot records that the server started, and whether the run before it
// ended without shutting down (a crash, the machine losing power, being
// killed for running out of memory): that run's last entry says when it
// was last seen alive.
func (s *Store) Boot(ctx context.Context) {
	if s == nil {
		return
	}
	last, ok, err := s.Last(ctx)
	attrs := map[string]any{}
	if err == nil && ok && last.Kind != KindShutdown {
		attrs["previous"] = "no clean shutdown"
		attrs["last_seen"] = last.Time.UnixMilli()
	}
	s.Add(Entry{Kind: KindBoot, Level: slog.LevelInfo, Msg: "server started", Attrs: attrs})
}

// Shutdown records that the server stops cleanly. Close comes after it.
func (s *Store) Shutdown() {
	s.Add(Entry{Kind: KindShutdown, Level: slog.LevelInfo, Msg: "server stopped"})
}
