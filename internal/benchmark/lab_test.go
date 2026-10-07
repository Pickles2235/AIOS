package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLabDemoHonestOutcomesAndParity(t *testing.T) {
	root := t.TempDir()
	db, e := BuildDemo(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	r, e := RunLab(context.Background(), db, root, "demo", DemoCases())
	if e != nil {
		t.Fatal(e)
	}
	if r.SourceSnapshotSHA256 != r.BaselineSnapshotSHA256 || len(r.Manifest) != 5 {
		t.Fatalf("parity: %+v", r.Manifest)
	}
	wins, unknown, wrong := 0, 0, 0
	for _, row := range r.Results {
		t.Log(row.Case.ID, row.KBState, row.KBCorrect, row.GrepCorrect, row.KB.Rank, row.Baseline.Rank)
		if row.GrepWins {
			wins++
		}
		if row.KBState == "unknown" && !row.KBCorrect {
			unknown++
		}
		if !row.KBCorrect {
			wrong++
		}
		if row.SourceBytesRead == 0 || row.SourceFilesRead != 5 || row.ContextBytes == 0 || row.EstimatedTokens != (row.ContextBytes+3)/4 {
			t.Fatal("missing counters")
		}
	}
	if wins == 0 || unknown == 0 || wrong == 0 {
		b, _ := json.MarshalIndent(r, "", " ")
		t.Fatalf("missing honest loss/unknown: %s", b)
	}
}
func TestLabRejectsCorruptSourceAndBounds(t *testing.T) {
	root := t.TempDir()
	db, e := BuildDemo(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.DB().Exec("UPDATE source_files SET content='corrupt' WHERE path='src/A.ts'"); e != nil {
		t.Fatal(e)
	}
	if _, e = RunLab(context.Background(), db, root, "demo", DemoCases()); e == nil {
		t.Fatal("corrupt canonical content accepted")
	}
	for _, cs := range [][]LabCase{nil, make([]LabCase, 41), {{Case: Case{ID: "x", Query: "x", ExpectedState: "found"}}}} {
		if ValidateLabCases(cs) == nil {
			t.Fatal("invalid cases accepted")
		}
	}
	if _, e = os.Stat(filepath.Join(root, "literal")); !os.IsNotExist(e) {
		t.Fatal("materialized corrupt source")
	}
}

func TestLabKeepsFailedRetrievalSeparateFromUnknownClassification(t *testing.T) {
	root := t.TempDir()
	db, e := BuildDemo(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := RunLab(ctx, db, root, "demo", DemoCases()); e == nil {
		t.Fatal("cancellation accepted as completed report")
	}
	c := DemoCases()[0]
	c.ExpectedPath = "docs/route.proto"
	if matchesLab(c, "demo", "src/ChargeRouter.ts", 1, 1, "routeCharge") {
		t.Fatal("wrong provenance accepted")
	}
	c.ExpectedPath = "src/ChargeRouter.ts"
	c.ExpectedLine = 2
	if matchesLab(c, "demo", "src/ChargeRouter.ts", 1, 1, "routeCharge") {
		t.Fatal("wrong line accepted")
	}
}
