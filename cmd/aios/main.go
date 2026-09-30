package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/adapter"
	"github.com/AdamNi-7080/AIOS/internal/app"
	"github.com/AdamNi-7080/AIOS/internal/benchmark"
	cachepkg "github.com/AdamNi-7080/AIOS/internal/cache"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	mcpserver "github.com/AdamNi-7080/AIOS/internal/mcp"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"github.com/AdamNi-7080/AIOS/internal/webui"
	"os"
	"strings"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "local":
		if len(args) < 2 || args[1] != "ingest" {
			return usageError()
		}
		fs := flag.NewFlagSet("local ingest", flag.ContinueOnError)
		config := fs.String("config", "", "source-policy catalog")
		registry := fs.String("registry", "", "approved local registry")
		data := fs.String("data-dir", "", "owned data directory")
		repo := fs.String("repo", "", "single approved repository ID")
		all := fs.Bool("all", false, "atomically bootstrap all approved local workspaces")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if *config == "" || *registry == "" || *data == "" {
			return fmt.Errorf("--config, --registry and --data-dir are required")
		}
		if (*repo == "") == !*all {
			return fmt.Errorf("exactly one of --repo or --all is required")
		}
		cfg, err := catalog.Load(*config)
		if err != nil {
			return err
		}
		reg, err := adapter.LoadLocalRegistry(*registry)
		if err != nil {
			return err
		}
		out, err := app.IngestLocal(ctx, cfg, reg, *data, *repo)
		if err != nil {
			return err
		}
		return writeJSON(out)
	case "ingest":
		fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
		config := fs.String("config", "", "mirror source-policy catalog JSON path")
		registry := fs.String("registry", "", "approved mirror registry JSON path")
		data := fs.String("data-dir", "", "agent-owned data directory")
		repo := fs.String("repo", "", "approved repository id")
		all := fs.Bool("all", false, "bootstrap every approved repository atomically")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *config == "" || *registry == "" || *data == "" {
			return fmt.Errorf("--config, --registry, and --data-dir are required")
		}
		if (*repo == "") == !*all {
			return fmt.Errorf("exactly one of --repo or --all is required")
		}
		if *all {
			out, err := app.IngestMirrorCatalog(ctx, *config, *registry, *data)
			if err != nil {
				return err
			}
			return writeJSON(out)
		}
		out, err := app.IngestMirrorRevision(ctx, *config, *registry, *data, *repo)
		if err != nil {
			return err
		}
		return writeJSON(out)
	case "mirrors":
		if len(args) < 2 || args[1] != "sync" {
			return usageError()
		}
		fs := flag.NewFlagSet("mirrors sync", flag.ContinueOnError)
		registry := fs.String("registry", "", "approved mirror registry JSON path")
		data := fs.String("data-dir", "", "agent-owned data directory")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if *registry == "" || *data == "" {
			return fmt.Errorf("--registry and --data-dir are required")
		}
		r, err := mirror.Load(*registry)
		if err != nil {
			return err
		}
		out, err := mirror.Sync(ctx, r, *data)
		if err != nil {
			return err
		}
		return writeJSON(map[string]any{"registry_fingerprint": mirror.Fingerprint(r), "mirrors": out})
	case "catalog":
		if len(args) < 2 || args[1] != "validate" {
			return usageError()
		}
		fs := flag.NewFlagSet("catalog validate", flag.ContinueOnError)
		config := fs.String("config", "", "catalog JSON path")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if *config == "" {
			return fmt.Errorf("--config is required")
		}
		cfg, err := catalog.Load(*config)
		if err != nil {
			return err
		}
		return writeJSON(map[string]any{"valid": true, "version": cfg.Version, "sources": len(cfg.Sources), "source_kinds": []string{"repository"}})
	case "status":
		fs := flag.NewFlagSet("status", flag.ContinueOnError)
		data := fs.String("data-dir", "", "agent-owned data directory")
		repo := fs.String("repo", "", "optional repository id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *data == "" {
			return fmt.Errorf("--data-dir is required")
		}
		db, err := store.OpenReadOnly(*data)
		if err != nil {
			return err
		}
		defer db.Close()
		out, err := db.Diagnostics(ctx, *repo)
		if err != nil {
			return err
		}
		cache, cacheErr := cachepkg.OpenReadOnly(*data)
		if cacheErr != nil {
			out.Cache = map[string]any{"state": "unavailable", "unavailable_reason": cacheErr.Error()}
		} else {
			defer cache.Close()
			out.Cache = cache.Diagnostics(ctx)
		}
		return writeJSON(out)
	case "projections":
		if len(args) < 2 {
			return usageError()
		}
		if args[1] == "reset" {
			fs := flag.NewFlagSet("projections reset", flag.ContinueOnError)
			data := fs.String("data-dir", "", "agent-owned data directory")
			kind := fs.String("kind", "", "projection kind; only vector is resettable")
			if err := fs.Parse(args[2:]); err != nil {
				return err
			}
			if *data == "" || *kind != "vector" {
				return fmt.Errorf("projections reset requires --data-dir and --kind vector")
			}
			db, err := store.OpenWriter(*data)
			if err != nil {
				return err
			}
			defer db.Close()
			if err = db.ResetVectorProjection(ctx); err != nil {
				return err
			}
			out, err := db.Diagnostics(ctx, "")
			if err != nil {
				return err
			}
			return writeJSON(out)
		}
		if args[1] != "rebuild" {
			return usageError()
		}
		fs := flag.NewFlagSet("projections rebuild", flag.ContinueOnError)
		data := fs.String("data-dir", "", "agent-owned data directory")
		config := fs.String("config", "", "optional catalog JSON path; required for vector")
		kind := fs.String("kind", "", "optional projection kind, or comma-separated kinds")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if *data == "" {
			return fmt.Errorf("--data-dir is required")
		}
		db, err := store.OpenWriter(*data)
		if err != nil {
			return err
		}
		defer db.Close()
		var kinds []string
		if *kind != "" {
			for _, value := range strings.Split(*kind, ",") {
				if value = strings.TrimSpace(value); value != "" {
					kinds = append(kinds, value)
				}
			}
		}
		vectorRequested := false
		var ordinary []string
		for _, value := range kinds {
			if value == "vector" {
				vectorRequested = true
			} else {
				ordinary = append(ordinary, value)
			}
		}
		if len(kinds) == 0 {
			ordinary = nil
		}
		if err = db.RebuildProjections(ctx, ordinary); err != nil {
			return err
		}
		if vectorRequested {
			if *config == "" {
				return fmt.Errorf("--config is required for vector projection rebuild")
			}
			cfg, e := catalog.Load(*config)
			if e != nil {
				return e
			}
			if e = db.RebuildVectorProjection(ctx, store.VectorOptions{Enabled: cfg.Vector.Enabled, Dimensions: cfg.Vector.Dimensions, DataDir: *data}, "operator_rebuild"); e != nil {
				return e
			}
		}
		out, err := db.Diagnostics(ctx, "")
		if err != nil {
			return err
		}
		return writeJSON(out)
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		config := fs.String("config", "", "catalog JSON path")
		data := fs.String("data-dir", "", "optional data directory")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *config == "" {
			return fmt.Errorf("--config is required")
		}
		out, err := app.Doctor(*config, *data)
		if err != nil {
			return err
		}
		return writeJSON(out)
	case "benchmark":
		fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
		fixture := fs.String("fixture", "", "checked-in benchmark fixture JSON path")
		data := fs.String("data-dir", "", "agent-owned derived data directory")
		output := fs.String("output", "", "optional report JSON output path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *fixture == "" || *data == "" {
			return fmt.Errorf("--fixture and --data-dir are required")
		}
		out, err := benchmark.Run(ctx, *fixture, *data)
		if err != nil {
			return err
		}
		if *output != "" {
			b, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return err
			}
			if err = os.WriteFile(*output, append(b, '\n'), 0600); err != nil {
				return err
			}
		}
		return writeJSON(out)
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		config := fs.String("config", "", "catalog JSON path")
		data := fs.String("data-dir", "", "agent-owned data directory")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *config == "" || *data == "" {
			return fmt.Errorf("--config and --data-dir are required")
		}
		cfg, err := catalog.Load(*config)
		if err != nil {
			return err
		}
		db, err := store.OpenReadOnly(*data)
		if err != nil {
			return err
		}
		defer db.Close()
		cache, cacheErr := cachepkg.Open(*data)
		if cacheErr != nil {
			cache = cachepkg.Disabled(cacheErr.Error())
		}
		defer cache.Close()
		return mcpserver.NewV1WithCache(cfg, db, cache).Run(ctx, &mcpserver.RecoveringStdioTransport{MaxFrameBytes: mcpserver.DefaultMaxFrameBytes})
	case "ui":
		if len(args) < 2 || args[1] != "serve" {
			return usageError()
		}
		fs := flag.NewFlagSet("ui serve", flag.ContinueOnError)
		config := fs.String("config", "", "catalog JSON path")
		data := fs.String("data-dir", "", "agent-owned data directory")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if *data == "" {
			return fmt.Errorf("--data-dir is required")
		}
		cfg := catalog.Config{Version: 1, Limits: catalog.Defaults()}
		var err error
		if *config != "" {
			cfg, err = catalog.Load(*config)
			if err != nil {
				return err
			}
		}
		writer, err := store.OpenWriter(*data)
		if err != nil {
			return err
		}
		if err = writer.Close(); err != nil {
			return err
		}
		db, err := store.OpenReadOnly(*data)
		if err != nil {
			return err
		}
		defer db.Close()
		server, err := webui.New(cfg, db, nil)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Open local UI:", server.URL())
		return server.Serve(ctx)
	default:
		return usageError()
	}
}
func writeJSON(value any) error { return json.NewEncoder(os.Stdout).Encode(value) }
func usageError() error {
	return fmt.Errorf("usage: aios <catalog validate|mirrors sync|local ingest (--all|--repo ID)|ingest (--all|--repo ID)|status|projections rebuild|doctor|benchmark|serve|ui serve --config CATALOG --data-dir DATA>")
}
