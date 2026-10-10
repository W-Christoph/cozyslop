package transfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cozycast/internal/store"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// server makes a data directory with one account and some files, and
// returns its export, taken while the database is open as in a running
// server.
func server(t *testing.T) []byte {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, Database))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CreateUser(ctx, &store.User{Username: "moved", PasswordHash: "hash", Nickname: "Moved"}); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "media/avatars/moved.png", "picture")
	write(t, dir, "wireguard.key", "key")
	if err := os.MkdirAll(filepath.Join(dir, "media", "chat"), 0o750); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Export(ctx, st, dir, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, exportTmp)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("work folder left behind: %v", err)
	}
	return out.Bytes()
}

func files(t *testing.T, dir string) string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if rel, _ := filepath.Rel(dir, p); err == nil && rel != "." && !strings.HasPrefix(filepath.Base(rel), Database+"-") {
			names = append(names, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(names, " ")
}

func TestExportImport(t *testing.T) {
	ctx := context.Background()
	archive := server(t)

	// The new server has run already: all of that goes.
	dir := t.TempDir()
	old, err := store.Open(ctx, filepath.Join(dir, Database))
	if err != nil {
		t.Fatal(err)
	}
	if err := old.CreateUser(ctx, &store.User{Username: "admin", PasswordHash: "hash", Nickname: "Admin"}); err != nil {
		t.Fatal(err)
	}
	old.Close()
	write(t, dir, "media/avatars/old.png", "old picture")
	write(t, dir, "certs/old", "old certificate")

	exported, err := Import(ctx, dir, bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	if exported.IsZero() {
		t.Fatal("no export time")
	}
	if got, want := files(t, dir), "cozycast.db media media/avatars media/avatars/moved.png media/chat wireguard.key"; got != want {
		t.Fatalf("data directory: %s, want %s", got, want)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "media", "avatars", "moved.png")); string(data) != "picture" {
		t.Fatalf("picture: %q", data)
	}
	st, err := store.Open(ctx, filepath.Join(dir, Database))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if u, err := st.UserByUsername(ctx, "moved"); err != nil || u.Nickname != "Moved" {
		t.Fatalf("account: %+v %v", u, err)
	}
	if _, err := st.UserByUsername(ctx, "admin"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the new server's own account stayed: %v", err)
	}
}

// archive makes a gzipped tar of the entries, name and content in turns; a
// name ending in "/" is a folder.
func archive(t *testing.T, entries ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for i := 0; i < len(entries); i += 2 {
		head := &tar.Header{Name: entries[i], Mode: 0o600, Size: int64(len(entries[i+1]))}
		if strings.HasSuffix(entries[i], "/") {
			head.Typeflag = tar.TypeDir
		}
		if err := tw.WriteHeader(head); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(entries[i+1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// Whatever is wrong with an archive, the data directory stays as it was.
func TestImportRefused(t *testing.T) {
	ctx := context.Background()
	good := server(t)
	info := func(format, schema int) string {
		data, _ := json.Marshal(manifest{Format: format, Schema: schema})
		return string(data)
	}
	now := info(format, store.SchemaVersion())
	for name, tc := range map[string]struct {
		data     []byte
		notOurs  bool
		contains string
	}{
		"not an archive":   {data: []byte("not an archive"), notOurs: true},
		"empty":            {notOurs: true},
		"a room's export":  {data: archive(t, "./", "", "./Desktop/", ""), notOurs: true},
		"cut off":          {data: good[:len(good)-20], contains: "cut off"},
		"cut off early":    {data: good[:len(good)/2], contains: "cut off"},
		"newer version":    {data: archive(t, manifestName, info(format, store.SchemaVersion()+1)), contains: "newer"},
		"newer format":     {data: archive(t, manifestName, info(format+1, 1)), contains: "newer"},
		"outside the data": {data: archive(t, manifestName, now, "../outside", "x"), notOurs: true},
		"absolute":         {data: archive(t, manifestName, now, "/etc/outside", "x"), notOurs: true},
		"no database":      {data: archive(t, manifestName, now, "wireguard.key", "key"), contains: "database"},
		"not a database":   {data: archive(t, manifestName, now, Database, "not a database"), contains: "database"},
		"twice the same":   {data: archive(t, manifestName, now, "a", "1", "a", "2"), contains: "exists"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "media/avatars/kept.png", "kept")
			_, err := Import(ctx, dir, bytes.NewReader(tc.data))
			if err == nil || errors.Is(err, ErrNotExport) != tc.notOurs || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("err = %v", err)
			}
			if got, want := files(t, dir), "media media/avatars media/avatars/kept.png"; got != want {
				t.Fatalf("data directory: %s, want %s", got, want)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "outside")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("written outside the data directory: %v", err)
			}
		})
	}
}
