package sqlite_test

import (
	"path/filepath"
	"testing"

	"codeplat/internal/core"
	"codeplat/internal/store/sqlite"
	"codeplat/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) core.Store {
		s, err := sqlite.Open(filepath.Join(t.TempDir(), "new-dir", "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	})
}
