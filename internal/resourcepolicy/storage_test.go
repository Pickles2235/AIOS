package resourcepolicy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedAdmissionMeasuresFilesAndRefusesBeforeBuild(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.db"), make([]byte, 64), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "index.db"), filepath.Join(root, "outside-link")); err != nil {
		t.Fatal(err)
	}
	s, err := CheckBudget(root, 256, 0, 64)
	if err != nil || s.OwnedBytes != 64 || s.MaxOwnedBytes != 256 || s.AvailableBytes <= 0 {
		t.Fatalf("wrong measured budget: %+v %v", s, err)
	}
	if err = os.WriteFile(filepath.Join(root, "stage"), make([]byte, 160), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = CheckBudget(root, 256, 0, 64)
	if !errors.Is(err, ErrStorageBudget) || s.StorageState != "budget" || s.OwnedBytes <= 192 {
		t.Fatalf("write admission not refused: %+v %v", s, err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "index.db")); len(got) != 64 {
		t.Fatal("budget probe changed canonical data")
	}
}
