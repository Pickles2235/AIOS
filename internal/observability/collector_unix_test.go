//go:build !windows

package observability

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sys/unix"
)

func TestPlantedContentNeverReachesLocalSinkOrArchive(t *testing.T) {
	data := t.TempDir()
	secret := "sk-test-PLANTED_SECRET_936"
	c, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	parent, err := trace.TraceIDFromHex("736b2d746573742d504c414e54454421")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: parent, SpanID: trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}, TraceFlags: trace.FlagsSampled}))
	_, span := c.Start(ctx, "query", map[string]string{"query": secret, "repository": "alice.private@example.invalid", "stage": secret})
	span.SetAttributes(attribute.String("path", "/Users/planted-private/source"), attribute.String("code", secret))
	span.AddEvent(secret)
	End(span, true)
	status, records, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !status.OTEL || !status.LocalOnly || !status.Correlation || len(records) != 2 || records[0].Outcome != "error" {
		t.Fatalf("status=%+v records=%+v", status, records)
	}
	for _, expanded := range []bool{false, true} {
		archive, err := c.Archive(expanded, map[string]any{"coverage": map[string]int{"active": 1}})
		if err != nil {
			t.Fatal(err)
		}
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range reader.File {
			f, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(b, []byte(secret)) || bytes.Contains(b, []byte("alice.private")) || bytes.Contains(b, []byte("/Users/planted")) || bytes.Contains(b, []byte(parent.String())) {
				t.Fatalf("unredacted %s", entry.Name)
			}
		}
	}
	b, err := os.ReadFile(filepath.Join(data, "diagnostics", "events-0.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(secret)) || bytes.Contains(b, []byte(parent.String())) {
		t.Fatal("raw identifier in sink")
	}
}

func TestSinkRejectsSymlinkHardlinkFIFOAndAnchorsAfterRename(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			data := t.TempDir()
			dir := filepath.Join(data, "diagnostics")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(target, []byte("sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "events-0.jsonl")
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(target, path)
			case "hardlink":
				err = os.Link(target, path)
			case "fifo":
				err = unix.Mkfifo(path, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			c, err := Open(data)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, span := c.Start(context.Background(), "query", nil)
			End(span, false)
			status, _, err := c.Snapshot()
			if err == nil || status.Available || status.Dropped == 0 {
				t.Fatalf("unsafe sink accepted: status=%+v err=%v", status, err)
			}
			outside, err := os.ReadFile(target)
			if err != nil || string(outside) != "sentinel" {
				t.Fatal("outside file changed")
			}
		})
	}
	data := t.TempDir()
	c, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	dir := filepath.Join(data, "diagnostics")
	old := dir + "-old"
	outside := t.TempDir()
	if err := os.Rename(dir, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dir); err != nil {
		t.Fatal(err)
	}
	_, span := c.Start(context.Background(), "query", nil)
	End(span, false)
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("symlink swap redirected output")
	}
	if _, err := os.Stat(filepath.Join(old, "events-0.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestBoundedRetentionAndReadOnlyRoot(t *testing.T) {
	data := t.TempDir()
	c, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if err := c.root.append(bytes.Repeat([]byte{'x'}, 2048), maxFile, maxFiles); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(data, "diagnostics"))
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	if len(entries) > maxFiles || total > maxFile*maxFiles {
		t.Fatalf("retention files=%d bytes=%d", len(entries), total)
	}
	root := t.TempDir()
	_, _, err = ReadExisting(root)
	if err == nil {
		t.Fatal("missing root unexpectedly readable")
	}
	if _, err = os.Stat(filepath.Join(root, "diagnostics")); !os.IsNotExist(err) {
		t.Fatal("read-only health created diagnostics")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe error")
	}
}
