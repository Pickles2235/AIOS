package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedCommandsAreNotExposed(t *testing.T) {
	err := run(context.Background(), []string{"runtime", "probe"})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("error=%v", err)
	}
}

func TestCatalogValidateAcceptsDocumentedCommandShape(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "catalog.json")
	body := `{"version":1,"sources":[`
	for i := 0; i < 25; i++ {
		if i > 0 {
			body += ","
		}
		body += `{"kind":"repository","id":"repo-` + fmt.Sprintf("%02d", i) + `"}`
	}
	body += `]}`
	if err := os.WriteFile(config, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"catalog", "validate", "--config", config}); err != nil {
		t.Fatal(err)
	}
}

func TestIngestRequiresExactlyOneScope(t *testing.T) {
	for _, args := range [][]string{
		{"ingest", "--config", "c", "--registry", "r", "--data-dir", "d"},
		{"ingest", "--config", "c", "--registry", "r", "--data-dir", "d", "--repo", "one", "--all"},
	} {
		err := run(context.Background(), args)
		if err == nil || !strings.Contains(err.Error(), "exactly one of --repo or --all") {
			t.Fatalf("args=%v error=%v", args, err)
		}
	}
}
