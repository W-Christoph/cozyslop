package diag

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DumpDir is the folder beside the database (its name without ".db") that
// holds the stall reports' goroutine dumps: too big for the database, and
// written even when it is stuck.
func DumpDir(database string) string {
	return strings.TrimSuffix(database, ".db")
}

// Stall reports, and what Go wrote when it ended the server (Boot).
var dumpName = regexp.MustCompile(`^(stall|crash)-[0-9]+\.txt\.gz$`)

// ErrNoDump is the answer for a dump that does not exist.
var ErrNoDump = errors.New("diagnostics: no such dump")

// writeDump writes a gzipped dump named name with write.
func (s *Store) writeDump(name string, write func(io.Writer) error) error {
	if s == nil || s.dumps == "" {
		return errors.New("diagnostics: dumps are not kept")
	}
	if err := os.MkdirAll(s.dumps, 0o750); err != nil {
		return err
	}
	tmp := filepath.Join(s.dumps, "."+name+".tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	gz := gzip.NewWriter(f)
	err = errors.Join(write(gz), gz.Close(), f.Close())
	if err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dumps, name))
}

// Dump opens the dump named name, as text.
func (s *Store) Dump(name string) (io.ReadCloser, error) {
	if s == nil || s.dumps == "" || !dumpName.MatchString(name) {
		return nil, ErrNoDump
	}
	f, err := os.Open(filepath.Join(s.dumps, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoDump
	}
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return struct {
		io.Reader
		io.Closer
	}{gz, f}, nil
}

// removeDumps removes the dumps last changed before t (all with zero t).
func (s *Store) removeDumps(t time.Time) {
	if s.dumps == "" {
		return
	}
	list, _ := os.ReadDir(s.dumps)
	for _, e := range list {
		info, err := e.Info()
		if err != nil || !t.IsZero() && info.ModTime().After(t) {
			continue
		}
		if dumpName.MatchString(e.Name()) || strings.HasSuffix(e.Name(), ".tmp") {
			os.Remove(filepath.Join(s.dumps, e.Name()))
		}
	}
}
