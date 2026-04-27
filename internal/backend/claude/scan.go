package claude

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justincordova/seshr/internal/session"
)

// claudeMeta is lightweight Claude-specific metadata produced by scanDir.
type claudeMeta struct {
	ID         string
	Path       string
	Project    string
	Source     session.SourceKind
	Size       int64
	ModifiedAt time.Time
	HasBackup  bool
	TurnCount  int
	TokenCount int
}

// scanDir walks one level deep into root and returns Claude-specific metadata
// for every .jsonl file found. A missing root is treated as "no sessions".
// Results are sorted by ModifiedAt descending (most-recent first).
func scanDir(root string) ([]claudeMeta, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", root, err)
	}

	var out []claudeMeta
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
			out = append(out, claudeMeta{
				ID:         strings.TrimSuffix(pe.Name(), ".jsonl"),
				Path:       filepath.Join(projDir, pe.Name()),
				Project:    e.Name(),
				Source:     session.SourceClaude,
				Size:       info.Size(),
				ModifiedAt: info.ModTime(),
				HasBackup:  backups[pe.Name()],
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ModifiedAt.After(out[j].ModifiedAt)
	})

	for i := range out {
		out[i].TokenCount = estimateTokensFromSize(out[i].Size)
		if out[i].TokenCount > 0 {
			out[i].TurnCount = out[i].TokenCount / 3000
			if out[i].TurnCount < 1 {
				out[i].TurnCount = 1
			}
		}
	}

	return out, nil
}

// estimateTokensFromSize produces a rough token count from file size. The
// tokenizer heuristic is rune_count/3.5 for text content. JSONL overhead
// (keys, braces, timestamps) inflates byte count relative to actual content,
// so dividing by 5 gives a reasonable ballpark for the picker display. Exact
// counts are computed when the user opens a session via Parse.
func estimateTokensFromSize(size int64) int {
	if size <= 0 {
		return 0
	}
	return int(size / 5)
}
