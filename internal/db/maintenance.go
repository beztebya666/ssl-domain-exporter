package db

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BackupFile struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

func (d *DB) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

func (d *DB) Ping() error {
	if d == nil || d.sql == nil {
		return fmt.Errorf("database is not initialized")
	}
	return d.sql.Ping()
}

func (d *DB) BackupTo(dest string) error {
	if d == nil || d.sql == nil {
		return fmt.Errorf("database is not initialized")
	}
	dest = filepath.Clean(strings.TrimSpace(dest))
	if dest == "" {
		return fmt.Errorf("backup destination is required")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	_ = os.Remove(dest)
	// SQLite does not support query parameters for VACUUM INTO, so the file path
	// has to be embedded in the statement after single-quote escaping.
	//nolint:gosec // SQLite VACUUM INTO does not support parameters; the path is single-quote escaped above.
	statement := fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(dest, "'", "''"))
	if _, err := d.sql.Exec(statement); err != nil {
		return fmt.Errorf("create sqlite backup: %w", err)
	}
	return nil
}

func (d *DB) DeleteChecksOlderThan(cutoff time.Time) (int64, error) {
	if d == nil || d.sql == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	result, err := d.sql.Exec(`DELETE FROM domain_checks WHERE checked_at < ?`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *DB) CountChecksOlderThan(cutoff time.Time) (int, error) {
	if d == nil || d.sql == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var count int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM domain_checks WHERE checked_at < ?`, cutoff.UTC()).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func ListBackupFiles(dir string) ([]BackupFile, error) {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" {
		return []BackupFile{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupFile{}, nil
		}
		return nil, err
	}
	files := make([]BackupFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		name := entry.Name()
		if !isBackupExtension(name) {
			continue
		}
		files = append(files, BackupFile{
			Name:       name,
			Path:       filepath.Join(dir, name),
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt.After(files[j].ModifiedAt)
	})
	return files, nil
}

func RestoreSQLiteFile(src, dest string) error {
	src = filepath.Clean(strings.TrimSpace(src))
	dest = filepath.Clean(strings.TrimSpace(dest))
	if src == "" || dest == "" {
		return fmt.Errorf("source and destination are required")
	}
	input, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open backup source: %w", err)
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create restore destination dir: %w", err)
	}

	tmp := dest + ".restore.tmp"
	output, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create restore temp file: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("copy backup file: %w", err)
	}
	if err := output.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close restore temp file: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace sqlite database: %w", err)
	}
	return nil
}

func isBackupExtension(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".bak")
}
