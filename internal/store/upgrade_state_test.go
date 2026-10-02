package store

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUpgradeSnapshotLeaseDoesNotMigrateAndStagedMigrationPreservesCanonicalRows(t *testing.T) {
	data, g := populatedV9(t)
	ctx := context.Background()
	p := filepath.Join(data, DatabaseName)
	bytes, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	original := sha256.Sum256(bytes)
	l, e := AcquireDataLease(data)
	if e != nil {
		t.Fatal(e)
	}
	before, e := ReadUpgradeState(ctx, data)
	if e != nil {
		t.Fatal(e)
	}
	if before.Format != "knowledge-ir-v9" || before.ActiveGenerations["repo"] != g.ID {
		t.Fatal("prior state changed")
	}
	if _, e = OpenWriter(data); e != ErrWriterLocked {
		t.Fatal("snapshot writer isolation", e)
	}
	l.Close()
	bytes, e = os.ReadFile(p)
	if e != nil || sha256.Sum256(bytes) != original {
		t.Fatal("lease-only snapshot migrated old data")
	}
	stage := t.TempDir()
	os.Chmod(stage, 0700)
	if e = os.WriteFile(filepath.Join(stage, DatabaseName), bytes, 0600); e != nil {
		t.Fatal(e)
	}
	after, e := MigrateUpgradeState(ctx, stage)
	if e != nil {
		t.Fatal(e)
	}
	if after.Format != format || after.CanonicalFingerprint != before.CanonicalFingerprint || !reflect.DeepEqual(after.ActiveGenerations, before.ActiveGenerations) || len(after.RebuiltProjections) != 0 {
		t.Fatal("compatible projections or canonical IR replaced", after)
	}
	bytes, _ = os.ReadFile(p)
	if sha256.Sum256(bytes) != original {
		t.Fatal("staged migration changed original")
	}
	db, e := OpenWriter(stage)
	if e != nil {
		t.Fatal(e)
	}
	var graph, lookup string
	db.DB().QueryRow(`SELECT projection_build_id FROM active_projection_builds WHERE projection_kind='graph'`).Scan(&graph)
	db.DB().QueryRow(`SELECT projection_build_id FROM active_projection_builds WHERE projection_kind='lookup'`).Scan(&lookup)
	_, e = db.DB().Exec(`UPDATE projection_builds SET builder_version='previous' WHERE projection_build_id=?`, graph)
	db.Close()
	if e != nil {
		t.Fatal(e)
	}
	repaired, e := MigrateUpgradeState(ctx, stage)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(repaired.RebuiltProjections, []string{"graph"}) || repaired.CanonicalFingerprint != before.CanonicalFingerprint {
		t.Fatal("changed-only repair", repaired)
	}
	db, e = OpenReadOnly(stage)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var nextGraph, nextLookup string
	db.DB().QueryRow(`SELECT projection_build_id FROM active_projection_builds WHERE projection_kind='graph'`).Scan(&nextGraph)
	db.DB().QueryRow(`SELECT projection_build_id FROM active_projection_builds WHERE projection_kind='lookup'`).Scan(&nextLookup)
	if nextGraph == graph || nextLookup != lookup {
		t.Fatal("repair replaced unchanged projection")
	}
}
