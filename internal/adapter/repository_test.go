package adapter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestRepositoryGitDiscoversOnlyAnApprovedMaterialisedRevision(t *testing.T) {
	remote := t.TempDir()
	run(t, remote, "init", "-q", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(remote, "a.txt"), []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, remote, "add", ".")
	run(t, remote, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	registry := mirror.Registry{Version: mirror.RegistryVersion}
	for i := 0; i < 25; i++ {
		registry.Repositories = append(registry.Repositories, mirror.Repository{ID: "repo" + string(rune('a'+i)), URL: remote, Ref: "refs/heads/main"})
	}
	data := t.TempDir()
	if _, err := mirror.Sync(context.Background(), registry, data); err != nil {
		t.Fatal(err)
	}
	discovery, err := (RepositoryGit{Registry: registry, DataDir: data}).Discover(context.Background(), "repoa")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(discovery.Root, 0700)
	if discovery.Identity.Kind != model.SourceKindRepository || discovery.Identity.AdapterVersion != model.RepositoryAdapterVersion || discovery.Revision == "" {
		t.Fatalf("discovery=%#v", discovery)
	}
	if body, err := os.ReadFile(filepath.Join(discovery.Root, "a.txt")); err != nil || string(body) != "one" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
