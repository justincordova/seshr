package opencode

import (
	"context"
	"os"
	"testing"
)

// BenchmarkScanRealDB runs Scan against the user's real OpenCode database
// when available ($SESHR_OC_DB or ~/.local/share/opencode/opencode.db). The
// plan (design §4.2) targets < 500ms for the author's 1290-session DB.
//
// Skipped when the DB is not present so CI / other machines don't fail.
func BenchmarkScanRealDB(b *testing.B) {
	path := os.Getenv("SESHR_OC_DB")
	if path == "" {
		p, err := DefaultDBPath()
		if err != nil {
			b.Skip("no home dir available")
		}
		path = p
	}
	if _, err := os.Stat(path); err != nil {
		b.Skipf("no OpenCode DB at %s", path)
	}

	store, err := NewStore(path, b.TempDir())
	if err != nil {
		b.Fatalf("open store: %v", err)
	}
	b.Cleanup(func() { _ = store.Close() })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metas, err := store.Scan(context.Background())
		if err != nil {
			b.Fatalf("scan: %v", err)
		}
		if len(metas) == 0 {
			b.Fatal("expected non-empty result")
		}
	}
}
