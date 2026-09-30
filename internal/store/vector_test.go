package store

import (
	"context"
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/vector"
)

func TestVectorProjectionBuildsReusesAndResolvesCanonicalEvidence(t *testing.T) {
	ctx := context.Background()
	db, err := OpenWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	g := canonicalFixture(t, db, "one")
	if err = db.ActivateGeneration(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	options := VectorOptions{Enabled: true, Embedder: vector.NewLocal("fixture-vector", 16)}
	if err = db.RebuildVectorProjection(ctx, options, "test"); err != nil {
		t.Fatal(err)
	}
	first := ProjectionStatus{}
	for _, p := range mustDiagnostics(t, db).Projections {
		if p.Kind == "vector" {
			first = p
		}
	}
	if first.State != "ready" || first.RecordCounts["records"] == nil {
		t.Fatalf("status=%#v", first)
	}
	hits, scores, err := db.VectorCandidates(ctx, "customer", QueryFilter{Repository: "repo"}, options.Embedder, 10)
	if err != nil || len(hits) == 0 || len(scores) != len(hits) || hits[0].Evidence.ID == "" {
		t.Fatalf("hits=%#v scores=%#v err=%v", hits, scores, err)
	}
	if err = db.RebuildVectorProjection(ctx, options, "test_rebuild"); err != nil {
		t.Fatal(err)
	}
	second := ProjectionStatus{}
	for _, p := range mustDiagnostics(t, db).Projections {
		if p.Kind == "vector" {
			second = p
		}
	}
	if second.RecordCounts["reused"] == nil {
		t.Fatalf("rebuild did not report reuse: %#v", second)
	}
	if err = db.RebuildVectorProjection(ctx, VectorOptions{}, "disabled"); err != nil {
		t.Fatal(err)
	}
	if err = db.RequireProjection(ctx, "vector"); err == nil {
		t.Fatal("disabled vector projection was readable")
	}
}

func mustDiagnostics(t *testing.T, db *Store) Status {
	t.Helper()
	d, err := db.Diagnostics(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	return d
}
