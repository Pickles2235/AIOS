package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestCloudCanonicalMembershipAndPaging(t *testing.T) {
	db, s := fixture(t)
	defer db.Close()
	ctx := context.Background()
	estate, err := s.Cloud(ctx, "", "", "", 1)
	if err != nil || estate.TotalNodes != 1 || estate.TotalFiles != 1 || estate.TotalEntities < 2 || len(estate.Nodes) != 1 {
		t.Fatalf("estate=%+v err=%v", estate, err)
	}
	repo, err := s.Cloud(ctx, estate.Nodes[0].ChildScope, estate.Snapshot, "", 1)
	if err != nil || repo.TotalNodes != 1 || len(repo.Nodes) != 1 || repo.Nodes[0].Kind != "path_group" {
		t.Fatalf("repo=%+v err=%v", repo, err)
	}
	if repo.Nodes[0].MemberCount != repo.TotalEntities {
		t.Fatalf("evidence-path membership mismatch: group=%d repository=%d", repo.Nodes[0].MemberCount, repo.TotalEntities)
	}
	group, err := s.Cloud(ctx, repo.Nodes[0].ChildScope, repo.Snapshot, "", 1)
	if err != nil || group.TotalFiles != 1 || group.TotalEntities < 2 || len(group.Nodes) != 1 {
		t.Fatalf("group=%+v err=%v", group, err)
	}
	if group.Nodes[0].MemberCount != group.TotalEntities {
		t.Fatalf("file entity membership mismatch: file=%d group=%d", group.Nodes[0].MemberCount, group.TotalEntities)
	}
	file, err := s.Cloud(ctx, group.Nodes[0].ChildScope, group.Snapshot, "", 1)
	if err != nil || file.TotalNodes < 2 || len(file.Nodes) != 1 || file.NextCursor == "" || file.EdgeCount != 0 {
		t.Fatalf("file=%+v err=%v", file, err)
	}
	next, err := s.Cloud(ctx, file.Scope, file.Snapshot, file.NextCursor, 1)
	if err != nil || len(next.Nodes) != 1 || next.Nodes[0].Handle == file.Nodes[0].Handle {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	if _, err = s.Cloud(ctx, file.Scope, "wrong", file.NextCursor, 1); !errors.Is(err, ErrCloudStale) {
		t.Fatalf("stale snapshot: %v", err)
	}
	focused, err := s.Cloud(ctx, "", file.Snapshot, "", 1, next.Nodes[0].Handle)
	if err != nil || focused.Focus != next.Nodes[0].Handle || len(focused.Nodes) != 1 || focused.Nodes[0].Handle != next.Nodes[0].Handle {
		t.Fatalf("focus=%+v err=%v", focused, err)
	}
	old := file.Snapshot
	f := model.File{RepoID: "repo", Path: "new.go", SHA256: "two", Size: 1, Language: "go", Classification: "source", Content: "x"}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: 1, TotalBytes: 1, ExtractorVersions: model.ExtractorVersion}, []model.File{f}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Cloud(ctx, file.Scope, old, "", 1); !errors.Is(err, ErrCloudStale) {
		t.Fatalf("old snapshot: %v", err)
	}
	if _, err = s.Cloud(ctx, file.Scope, "", "", 1); !errors.Is(err, ErrCloudStale) {
		t.Fatalf("removed file: %v", err)
	}
}

func TestCloudNestedCapturedModulesPartitionMembership(t *testing.T) {
	db, s := fixture(t)
	defer db.Close()
	ctx := context.Background()
	files := []model.File{
		{RepoID: "repo", Path: "packages/a/package.json", SHA256: "a-manifest", Size: 12, Language: "json", Classification: "source", Content: `{"name":"a"}`},
		{RepoID: "repo", Path: "packages/a/src/a.go", SHA256: "a", Size: 1, Language: "go", Classification: "source", Content: "a"},
		{RepoID: "repo", Path: "packages/b/package.json", SHA256: "b-manifest", Size: 12, Language: "json", Classification: "source", Content: `{"name":"b"}`},
		{RepoID: "repo", Path: "packages/b/src/b.go", SHA256: "b", Size: 1, Language: "go", Classification: "source", Content: "b"},
		{RepoID: "repo", Path: "packages/readme.md", SHA256: "readme", Size: 1, Language: "markdown", Classification: "documentation", Content: "r"},
	}
	span := model.Span{StartByte: 0, EndByte: 1, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2}
	edges := []model.Edge{
		{RepoID: "repo", Path: "packages/a/package.json", Source: "<file>", Target: "a", Kind: "DECLARES_MODULE", Span: span, Resolver: "package-json", Confidence: 1},
		{RepoID: "repo", Path: "packages/b/package.json", Source: "<file>", Target: "b", Kind: "DECLARES_MODULE", Span: span, Resolver: "package-json", Confidence: 1},
	}
	g, err := db.StageGeneration(ctx, model.Snapshot{RepoID: "repo", Root: "/repo", Git: model.GitState{Commit: "two"}, ContentHash: "two", FileCount: len(files), IndexedAt: time.Now(), ExtractorVersions: model.ExtractorVersion}, files, nil, edges)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	estate, err := s.Cloud(ctx, "", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := s.Cloud(ctx, estate.Nodes[0].ChildScope, estate.Snapshot, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if repo.TotalNodes != 3 || repo.TotalFiles != 5 {
		t.Fatalf("repo groups: %+v", repo)
	}
	want := map[string]int{"packages/a": 2, "packages/b": 2, "packages": 1}
	for _, node := range repo.Nodes {
		if want[node.Path] != node.FileCount {
			t.Fatalf("group %q count=%d", node.Path, node.FileCount)
		}
		group, e := s.Cloud(ctx, node.ChildScope, repo.Snapshot, "", 100)
		if e != nil {
			t.Fatal(e)
		}
		if group.TotalFiles != want[node.Path] || len(group.Nodes) != want[node.Path] {
			t.Fatalf("group %+v", group)
		}
		delete(want, node.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing groups: %+v", want)
	}
}
