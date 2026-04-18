package parser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionMeta is lightweight metadata produced by Scan. The heavy fields
// (turn count, token total, topics) are filled in by Parse when the user
// actually opens the session.
type SessionMeta struct {
	ID         string // filename stem (no .jsonl extension)
	Path       string // absolute path to the .jsonl file
	Project    string // parent directory name, e.g. "-Users-j-myproject"
	Source     Source
	Size       int64
	ModifiedAt time.Time
	HasBackup  bool // true if a sibling <Path>.bak exists — see SPEC §4.5
	TurnCount  int  // populated by quick parse during Scan
	TokenCount int  // populated by quick parse during Scan
}

// Scan walks one level deep into root and returns metadata for every
// .jsonl file found. A missing root is treated as "no sessions" rather
// than an error, since that's the first-run case. Results are sorted by
// ModifiedAt descending (most-recent first).
func Scan(root string) ([]SessionMeta, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", root, err)
	}

	var out []SessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projDir := filepath.Join(root, e.Name())
		projEntries, err := os.ReadDir(projDir)
		if err != nil {
			slog.Warn("skipping project dir", "path", projDir, "err", err)
			continue
		}
		// Index of .bak files keyed by the name they back up.
		backups := map[string]bool{}
		for _, pe := range projEntries {
			if !pe.IsDir() && strings.HasSuffix(pe.Name(), ".jsonl.bak") {
				backups[strings.TrimSuffix(pe.Name(), ".bak")] = true
			}
		}
		for _, pe := range projEntries {
			if pe.IsDir() || !strings.HasSuffix(pe.Name(), ".jsonl") {
				continue
			}
			info, err := pe.Info()
			if err != nil {
				slog.Warn("stat jsonl", "path", pe.Name(), "err", err)
				continue
			}
			out = append(out, SessionMeta{
				ID:         strings.TrimSuffix(pe.Name(), ".jsonl"),
				Path:       filepath.Join(projDir, pe.Name()),
				Project:    e.Name(),
				Source:     SourceClaude,
				Size:       info.Size(),
				ModifiedAt: info.ModTime(),
				HasBackup:  backups[pe.Name()],
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ModifiedAt.After(out[j].ModifiedAt)
	})

	p := NewClaude()
	for i := range out {
		sess, err := p.Parse(context.Background(), out[i].Path)
		if err != nil {
			slog.Warn("quick parse failed", "path", out[i].Path, "err", err)
			continue
		}
		out[i].TurnCount = len(sess.Turns)
		out[i].TokenCount = sess.TokenCount
	}

	return out, nil
}
