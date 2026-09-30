package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/model"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVariableEstateMatchingAndAtomicBootstrap(t *testing.T) {
	for _, n := range []int{1, 3, 25, 100} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			root := t.TempDir()
			t.Cleanup(func() {
				_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
					if err == nil && d.IsDir() {
						return os.Chmod(path, 0700)
					}
					return err
				})
			})
			source := filepath.Join(root, "source")
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
			run := func(args ...string) {
				t.Helper()
				if out, err := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("git: %v %s", err, out)
				}
			}
			run("init", "-q", "--initial-branch=main")
			if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("immutable fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			run("add", ".")
			run("-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "fixture")
			cfg := catalog.Config{Version: 1, Limits: catalog.Defaults()}
			reg := mirror.Registry{Version: 1}
			for i := 0; i < n; i++ {
				id := fmt.Sprintf("repo-%03d", i)
				cfg.Sources = append(cfg.Sources, catalog.Source{Kind: model.SourceKindRepository, ID: id})
				reg.Repositories = append(reg.Repositories, mirror.Repository{ID: id, URL: source, Ref: "refs/heads/main"})
			}
			configPath := filepath.Join(root, "catalog.json")
			registryPath := filepath.Join(root, "registry.json")
			write := func(path string, v any) {
				t.Helper()
				b, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(configPath, cfg)
			write(registryPath, reg)
			if _, _, _, err := loadMirrorInputs(configPath, registryPath); err != nil {
				t.Fatal(err)
			}
			if n <= 3 {
				data := filepath.Join(root, "data")
				if _, err := mirror.Sync(context.Background(), reg, data); err != nil {
					t.Fatal(err)
				}
				if _, err := IngestMirrorCatalog(context.Background(), configPath, registryPath, data); err != nil {
					t.Fatal(err)
				}
				db, err := store.OpenReadOnly(data)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				snapshots, err := db.Status(context.Background(), "")
				if err != nil || len(snapshots) != n {
					t.Fatalf("bootstrap %d: %v %v", n, snapshots, err)
				}
			}
			reg.Repositories[0].ID = "unknown"
			write(registryPath, reg)
			if _, _, _, err := loadMirrorInputs(configPath, registryPath); err == nil {
				t.Fatal("mismatched IDs accepted")
			}
			reg.Repositories = reg.Repositories[:n-1]
			write(registryPath, reg)
			if _, _, _, err := loadMirrorInputs(configPath, registryPath); err == nil {
				t.Fatal("mismatched counts accepted")
			}
		})
	}
}
