package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const gzSuffix = ".gz"

// rotateFiles shifts numbered backups (name.1.ext → name.2.ext, etc.),
// evicts any whose number exceeds maxBackups, and renames the current
// basePath to name.1.ext. When compress is true, the newly-rotated
// name.1.ext is gzipped to name.1.ext.gz after the rename.
//
// Backup names may be either "name.N.ext" (plaintext) or "name.N.ext.gz"
// (already-compressed from a previous rotation with compression on);
// both forms participate in the shift.
func rotateFiles(basePath string, maxBackups int, compress bool) error {
	dir := filepath.Dir(basePath)
	base := filepath.Base(basePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	backups, err := findBackups(dir, name, ext)
	if err != nil {
		return err
	}

	sort.Sort(sort.Reverse(sort.IntSlice(backupNums(backups))))

	for _, b := range sortBackupsDesc(backups) {
		if b.num >= maxBackups {
			os.Remove(filepath.Join(dir, b.filename))
			continue
		}
		oldPath := filepath.Join(dir, b.filename)
		newName := fmt.Sprintf("%s.%d%s", name, b.num+1, ext)
		if b.compressed {
			newName += gzSuffix
		}
		newPath := filepath.Join(dir, newName)
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("failed to rotate %s to %s: %w", oldPath, newPath, err)
		}
	}

	if _, err := os.Stat(basePath); err != nil {
		// Nothing to rotate in. Can happen if the first write hasn't occurred.
		return nil
	}

	rotatedPath := filepath.Join(dir, fmt.Sprintf("%s.1%s", name, ext))
	if err := os.Rename(basePath, rotatedPath); err != nil {
		return fmt.Errorf("failed to rotate current log: %w", err)
	}

	if compress {
		if err := gzipFile(rotatedPath); err != nil {
			// Non-fatal: leave the uncompressed .1 file in place so log data
			// is never lost.
			fmt.Fprintf(os.Stderr, "log compression error for %s: %v\n", rotatedPath, err)
		}
	}

	return nil
}

type backupFile struct {
	num        int
	compressed bool
	filename   string
}

func backupNums(bs []backupFile) []int {
	out := make([]int, len(bs))
	for i, b := range bs {
		out[i] = b.num
	}
	return out
}

func sortBackupsDesc(bs []backupFile) []backupFile {
	out := make([]backupFile, len(bs))
	copy(out, bs)
	sort.Slice(out, func(i, j int) bool { return out[i].num > out[j].num })
	return out
}

func findBackups(dir, name, ext string) ([]backupFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var backups []backupFile
	prefix := name + "."
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fname := entry.Name()
		if !strings.HasPrefix(fname, prefix) {
			continue
		}

		compressed := false
		tail := fname
		if strings.HasSuffix(tail, gzSuffix) {
			compressed = true
			tail = strings.TrimSuffix(tail, gzSuffix)
		}
		if !strings.HasSuffix(tail, ext) {
			continue
		}

		numStr := strings.TrimSuffix(strings.TrimPrefix(tail, prefix), ext)
		num, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		backups = append(backups, backupFile{num: num, compressed: compressed, filename: fname})
	}

	return backups, nil
}

// gzipFile compresses src to src+".gz" and removes src on success. It does
// nothing if src doesn't exist. The written .gz has 0644 permissions.
func gzipFile(src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	dst := src + gzSuffix
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	gw := gzip.NewWriter(out)

	if _, err := io.Copy(gw, in); err != nil {
		gw.Close()
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := gw.Close(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}

	return os.Remove(src)
}

// pruneOldBackups deletes rotated backups older than maxAgeDays by mtime.
// The live log file at basePath is never touched.
func pruneOldBackups(basePath string, maxAgeDays int) error {
	if maxAgeDays <= 0 {
		return nil
	}

	dir := filepath.Dir(basePath)
	base := filepath.Base(basePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	backups, err := findBackups(dir, name, ext)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	for _, b := range backups {
		path := filepath.Join(dir, b.filename)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
	}
	return nil
}
