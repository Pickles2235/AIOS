package knowledge

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/store"
)

func TestHeldWriteLeasePreservesQueryBudgetAndNeverProvesAbsence(t *testing.T) {
	writer, original := fixture(t)
	defer writer.Close()
	reader, err := store.OpenReadOnly(filepath.Dir(writer.Path()))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	service := New(original.cfg, reader)
	before, err := service.Query(context.Background(), Query{Text: "Publish"})
	if err != nil || before.Status != "found" {
		t.Fatalf("fixture did not have actual canonical evidence: %+v %v", before, err)
	}
	if _, err = writer.DB().Exec("BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	defer writer.DB().Exec("ROLLBACK")
	for _, query := range []string{"Publish", "missing_symbol"} {
		start := time.Now()
		result, err := service.Query(context.Background(), Query{Text: query})
		if time.Since(start) > 500*time.Millisecond || result.Status == "not_found" || (err == nil && result.Status != "unknown") {
			t.Fatalf("locked canonical read exceeded budget or invented absence: %+v %v %s", result, err, time.Since(start))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	result, err := service.Query(ctx, Query{Text: "Publish"})
	if err != nil || result.Status != "unknown" || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("cancelled locked query did not return bounded unknown: %+v %v %s", result, err, time.Since(start))
	}
	if _, err = writer.DB().Exec("ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.Query(context.Background(), Query{Text: "Publish"})
	if err != nil || recovered.Status != "found" {
		t.Fatalf("contention lost last-good knowledge: %+v %v", recovered, err)
	}
}
