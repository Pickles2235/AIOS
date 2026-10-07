package adapter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/discover"
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

func TestRepositoryGitCaptureAllowsLargeExcludedTree(t *testing.T) {
	if mirror.MaxSnapshotFiles <= 200_000 {
		t.Fatal("full archive entry bound must exceed eligible catalog maximum")
	}
	source, data := t.TempDir(), t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(data, func(path string, info os.FileInfo, e error) error {
			if e == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return e
		})
	})
	run(t, source, "init", "-q", "--initial-branch=main")
	if err := os.MkdirAll(filepath.Join(source, "node_modules", "pkg"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "node_modules", "pkg", "blob.bin"), make([]byte, 2<<20), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(source, "node_modules", "pkg", fmt.Sprintf("extra-%d.txt", i)), []byte("ignored"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "Worker.java"), []byte("class Worker {}"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, source, "add", ".")
	run(t, source, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-qm", "one")
	registry := mirror.Registry{Version: mirror.RegistryVersion, Repositories: []mirror.Repository{{ID: "repo", URL: source, Ref: "refs/heads/main"}}}
	if _, err := mirror.Sync(context.Background(), registry, data); err != nil {
		t.Fatal(err)
	}
	discovery, err := (RepositoryGit{Registry: registry, DataDir: data}).Discover(context.Background(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	defer discovery.Close()
	limits := catalog.Defaults()
	limits.MaxTotalBytesPerRepo = 64 << 10
	limits.MaxFilesPerRepo = 1
	limits.MaxFileBytes = 64 << 10
	files, err := discover.Files(model.Repository{ID: "repo", Root: discovery.Root}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "Worker.java" {
		t.Fatalf("eligible files=%v", files)
	}
}
