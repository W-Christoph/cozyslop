// Package transfer moves a server's data to another server of this project:
// Export writes everything in the data directory (accounts, chat, room
// settings, pairings, uploaded pictures, the server's keys) as one archive,
// Import makes a data directory from such an archive. The rooms' desktops
// are not part of it; they move with "cozycast.sh export-room".
package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"cozycast/internal/store"
)

const (
	// Database is the database's file name in the data directory.
	Database = "cozycast.db"
	// The archive's first entry: what made it, and when.
	manifestName = "cozycast-server.json"
	format       = 1
	// Work folders in the data directory, never part of an archive.
	exportTmp = "export.tmp"
	importTmp = "import.tmp"
)

type manifest struct {
	Format   int       `json:"format"`
	Schema   int       `json:"schema"` // the database's migration level
	Exported time.Time `json:"exported"`
}

// ErrNotExport is returned for input that is not an archive Export made.
var ErrNotExport = errors.New("not a server export")

// Export writes the data directory as a gzipped tar to w. The server may be
// running: the database goes in as a copy taken in one piece (st is the
// database in dataDir), the files around it as they are found.
func Export(ctx context.Context, st *store.Store, dataDir string, w io.Writer) error {
	tmp := filepath.Join(dataDir, exportTmp)
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	snapshot := filepath.Join(tmp, Database)
	if err := st.Snapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("copy the database: %w", err)
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	info, err := json.Marshal(manifest{Format: format, Schema: store.SchemaVersion(), Exported: time.Now().UTC()})
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: manifestName, Mode: 0o600, Size: int64(len(info)), ModTime: time.Now()}); err != nil {
		return err
	}
	if _, err := tw.Write(info); err != nil {
		return err
	}
	if err := addFile(tw, snapshot, Database); err != nil {
		return err
	}
	err = filepath.WalkDir(dataDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // deleted meanwhile
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(dataDir, p)
		if err != nil || rel == "." {
			return err
		}
		name := filepath.ToSlash(rel)
		switch name {
		case exportTmp, importTmp:
			return filepath.SkipDir
		case Database, Database + "-wal", Database + "-shm", manifestName:
			return nil
		}
		switch {
		case d.IsDir():
			return tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: name + "/", Mode: 0o750})
		case d.Type().IsRegular():
			return addFile(tw, p, name)
		}
		return nil // links and the like: the server makes none
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addFile(tw *tar.Writer, file, name string) error {
	f, err := os.Open(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // deleted meanwhile
	}
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: st.Size(), ModTime: st.ModTime()}); err != nil {
		return err
	}
	if n, err := io.Copy(tw, io.LimitReader(f, st.Size())); err != nil {
		return err
	} else if n != st.Size() {
		return fmt.Errorf("%s changed while it was exported: try again", name)
	}
	return nil
}

// Import replaces what is in dataDir by the content of an archive Export
// made, and reports when that was made. The server must not be running.
// The archive is unpacked and checked in full first: one that is cut off,
// damaged or from a newer version leaves the data directory as it was.
func Import(ctx context.Context, dataDir string, r io.Reader) (exported time.Time, err error) {
	staging := filepath.Join(dataDir, importTmp)
	if err := os.RemoveAll(staging); err != nil {
		return exported, err
	}
	if err := os.Mkdir(staging, 0o700); err != nil {
		return exported, err
	}
	defer os.RemoveAll(staging)

	gz, err := gzip.NewReader(r)
	if err != nil {
		return exported, ErrNotExport
	}
	tr := tar.NewReader(gz)
	head, err := tr.Next()
	if err != nil || head.Name != manifestName {
		return exported, ErrNotExport
	}
	var info manifest
	if err := json.NewDecoder(io.LimitReader(tr, 1<<16)).Decode(&info); err != nil {
		return exported, ErrNotExport
	}
	if info.Format != format || info.Schema > store.SchemaVersion() {
		return exported, errors.New("the export was made by a newer CozyCast: update this server first")
	}
	cutOff := func(err error) error { return fmt.Errorf("the export is damaged or cut off: %w", err) }
	for {
		head, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return exported, cutOff(err)
		}
		if err := ctx.Err(); err != nil {
			return exported, err
		}
		name := path.Clean(head.Name)
		if !filepath.IsLocal(filepath.FromSlash(name)) || name == "." {
			return exported, fmt.Errorf("%w: entry %q", ErrNotExport, head.Name)
		}
		target := filepath.Join(staging, filepath.FromSlash(name))
		switch head.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o750)
		case tar.TypeReg:
			if err = writeFile(target, tr, head.ModTime); errors.Is(err, io.ErrUnexpectedEOF) {
				err = cutOff(err)
			}
		default:
			err = fmt.Errorf("%w: entry %q", ErrNotExport, head.Name)
		}
		if err != nil {
			return exported, err
		}
	}
	// The end of the tar is not the end of the file: its checksum follows.
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return exported, cutOff(err)
	}
	// Opening it checks that it is a database, and brings one from an older
	// version up to this one. (Opening a file that is not there makes one.)
	if _, err := os.Stat(filepath.Join(staging, Database)); err != nil {
		return exported, fmt.Errorf("the export's database: %w", err)
	}
	st, err := store.Open(ctx, filepath.Join(staging, Database))
	if err != nil {
		return exported, fmt.Errorf("the export's database: %w", err)
	}
	if err := st.Close(); err != nil {
		return exported, err
	}

	old, err := os.ReadDir(dataDir)
	if err != nil {
		return exported, err
	}
	for _, e := range old {
		if e.Name() == importTmp {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dataDir, e.Name())); err != nil {
			return exported, err
		}
	}
	fresh, err := os.ReadDir(staging)
	if err != nil {
		return exported, err
	}
	for _, e := range fresh {
		if err := os.Rename(filepath.Join(staging, e.Name()), filepath.Join(dataDir, e.Name())); err != nil {
			return exported, err
		}
	}
	return info.Exported, nil
}

func writeFile(target string, r io.Reader, modTime time.Time) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chtimes(target, modTime, modTime)
}
