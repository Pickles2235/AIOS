package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateUpgradeValidationCannotCreateOrMigrateInstalledData(t *testing.T) {
	root := t.TempDir()
	for _, data := range []string{filepath.Join(root, "data"), filepath.Join(root, ".upgrade-012345678901234567890123", "new-data")} {
		if e := runUpgrade(context.Background(), []string{"validate-state", "--data-dir", data, "--json"}); e == nil {
			t.Fatal("unowned validation accepted")
		}
		if _, e := os.Lstat(data); !os.IsNotExist(e) {
			t.Fatal("private validator created data", e)
		}
	}
}
