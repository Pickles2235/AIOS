package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLookupReadRejectsMissingRowsAndWrongEvidence(t *testing.T) {
	for _, mutation := range []string{
		`DELETE FROM projection_lookup_records WHERE entity_id=(SELECT entity_id FROM entities WHERE label='Publish' LIMIT 1)`,
		`UPDATE projection_lookup_records SET evidence_id=(SELECT evidence_id FROM evidence WHERE evidence_id<>projection_lookup_records.evidence_id LIMIT 1) WHERE entity_id=(SELECT entity_id FROM entities WHERE label='Publish' LIMIT 1)`,
	} {
		t.Run(mutation[:6], func(t *testing.T) {
			ctx := context.Background()
			db, e := OpenWriter(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			canonicalFixture(t, db, "one")
			before, e := ReadUpgradeState(ctx, filepath.Dir(db.Path()))
			if e != nil {
				t.Fatal(e)
			}
			if hits, e := db.ExactCandidates(ctx, "Publish", QueryFilter{}); e != nil || len(hits) == 0 {
				t.Fatal("valid exact result", e)
			}
			if _, e = db.DB().Exec(mutation); e != nil {
				t.Fatal(e)
			}
			if _, e = db.ExactCandidates(ctx, "Publish", QueryFilter{}); e == nil {
				t.Fatal("corrupt lookup certified an exact result")
			}
			if _, e = db.ExactCandidates(ctx, "missing_symbol", QueryFilter{}); e == nil {
				t.Fatal("corrupt lookup certified absence")
			}
			if problems, e := db.ValidateActiveProjectionProvenance(ctx); e != nil || len(problems) == 0 {
				t.Fatal("projection validation missed corruption", e)
			}
			if e = db.RebuildProjections(ctx, []string{"lookup"}); e != nil {
				t.Fatal(e)
			}
			after, e := ReadUpgradeState(ctx, filepath.Dir(db.Path()))
			if e != nil || after.CanonicalFingerprint != before.CanonicalFingerprint {
				t.Fatal("derived repair changed canonical IR", e)
			}
			if hits, e := db.ExactCandidates(ctx, "Publish", QueryFilter{}); e != nil || len(hits) == 0 {
				t.Fatal("exact repair did not restore result", e)
			}
		})
	}
}

func TestUpgradeLookupBuilderVersionChangesOnlyLookup(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	db, e := OpenWriter(data)
	if e != nil {
		t.Fatal(e)
	}
	canonicalFixture(t, db, "one")
	before, e := ReadUpgradeState(ctx, data)
	if e != nil {
		t.Fatal(e)
	}
	ids := map[string]string{}
	rows, e := db.DB().Query(`SELECT projection_kind,projection_build_id FROM active_projection_builds`)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var kind, id string
		if e = rows.Scan(&kind, &id); e != nil {
			t.Fatal(e)
		}
		ids[kind] = id
	}
	rows.Close()
	if _, e = db.DB().Exec(`UPDATE projection_builds SET builder_version='v1' WHERE projection_build_id=?`, ids["lookup"]); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExactCandidates(ctx, "Publish", QueryFilter{}); e == nil {
		t.Fatal("prior builder certified a v2 read")
	}
	db.Close()
	after, e := MigrateUpgradeState(ctx, data)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(after.RebuiltProjections, []string{"lookup"}) || after.CanonicalFingerprint != before.CanonicalFingerprint || !reflect.DeepEqual(after.ActiveGenerations, before.ActiveGenerations) {
		t.Fatal("upgrade did not preserve canonical state with only changed lookup", after)
	}
	db, e = OpenReadOnly(data)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	for kind, id := range ids {
		var next string
		if e = db.DB().QueryRow(`SELECT projection_build_id FROM active_projection_builds WHERE projection_kind=?`, kind).Scan(&next); e != nil {
			t.Fatal(e)
		}
		if (next == id) == (kind == "lookup") {
			t.Fatal("unexpected projection replacement", kind)
		}
	}
	if hits, e := db.ExactCandidates(ctx, "Publish", QueryFilter{}); e != nil || len(hits) == 0 {
		t.Fatal("v2 read unavailable after staged upgrade", e)
	}
}
