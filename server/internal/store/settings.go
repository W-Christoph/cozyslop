package store

import (
	"context"
	"database/sql"
)

// Settings are the global, admin-editable server settings.
type Settings struct {
	Message      string `json:"message"`      // shown on the front page
	Registration string `json:"registration"` // "open" or "invite"
}

var defaultSettings = Settings{Registration: "invite"}

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	out := defaultSettings
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		switch k {
		case "message":
			out.Message = v
		case "registration":
			out.Registration = v
		}
	}
	return out, rows.Err()
}

func (s *Store) SaveSettings(ctx context.Context, set Settings) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for k, v := range map[string]string{"message": set.Message, "registration": set.Registration} {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value", k, v)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
