package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestInstanceLegacyPersistenceResetAndRebuild(t *testing.T) {
	dir := canonicalTempDir(t)
	w, err := OpenWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	first, err := LoadInstance(dir)
	if err != nil || first.Name != "Homefold" || first.ID == "" {
		t.Fatalf("legacy default: %#v %v", first, err)
	}
	var b bytes.Buffer
	if err = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	saved, err := SaveInstance(dir, InstanceSettings{Name: "Research", SeedColour: "#AABBCC", Logo: "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())})
	if err != nil || saved.ID != first.ID {
		t.Fatalf("save: %#v %v", saved, err)
	}
	w, err = OpenWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	canonicalFixture(t, w, "first")
	if err = w.RebuildProjections(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	canonicalFixture(t, w, "incremental")
	w.Close()
	reopened, err := LoadInstance(dir)
	if err != nil || reopened != saved {
		t.Fatalf("restart/rebuild: %#v %v", reopened, err)
	}
	if err = ResetDerivedData(dir); err != nil {
		t.Fatal(err)
	}
	after, err := LoadInstance(dir)
	if err != nil || after != saved {
		t.Fatalf("reset: %#v %v", after, err)
	}
	info, err := os.Stat(filepath.Join(dir, instanceFile))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("metadata permissions")
	}
}
func TestInstanceRejectsUnsafeSettings(t *testing.T) {
	for _, v := range []InstanceSettings{{Name: "", SeedColour: "#123456"}, {Name: "ok", SeedColour: "red"}, {Name: "ok", SeedColour: "#123456", Logo: "https://example.test/logo.png"}, {Name: "ok", SeedColour: "#123456", Logo: "data:image/svg+xml;base64,PHN2Zz4="}, {Name: "ok", SeedColour: "#123456", Logo: "data:image/png;base64,YmFk"}} {
		if _, err := SaveInstance(canonicalTempDir(t), v); err == nil {
			t.Fatalf("accepted %#v", v)
		}
	}
	dir := canonicalTempDir(t)
	target := filepath.Join(canonicalTempDir(t), "private")
	os.WriteFile(target, []byte("unchanged"), 0600)
	os.Symlink(target, filepath.Join(dir, instanceFile))
	if _, err := LoadInstance(dir); err == nil {
		t.Fatal("followed metadata symlink")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "unchanged" {
		t.Fatal("modified symlink target")
	}
}

// macOS temporary directories may be reached through /var aliases. Product
// inputs require canonical paths; only these owned fixture paths are resolved.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
