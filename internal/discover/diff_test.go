package discover

import (
	"github.com/AdamNi-7080/AIOS/internal/model"
	"reflect"
	"testing"
)

func TestDiffClassifiesChangesWithoutRenameGuessing(t *testing.T) {
	old := []model.File{{Path: "gone.go", SHA256: "gone"}, {Path: "move.go", SHA256: "same"}, {Path: "same.go", SHA256: "old"}}
	now := []model.File{{Path: "new.go", SHA256: "new"}, {Path: "moved.go", SHA256: "same"}, {Path: "same.go", SHA256: "newer"}}
	want := []model.FileChange{{Kind: "removed", Path: "gone.go", SHA256: "gone"}, {Kind: "renamed", Path: "moved.go", OldPath: "move.go", SHA256: "same"}, {Kind: "added", Path: "new.go", SHA256: "new"}, {Kind: "modified", Path: "same.go", SHA256: "newer"}}
	if got := Diff(old, now); !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff()=%#v want %#v", got, want)
	}
}
