package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"github.com/AdamNi-7080/AIOS/internal/webui"
	"os"
	"path/filepath"
	"runtime/debug"
)

func runUpgrade(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("upgrade requires inspect|apply|recover")
	}
	fs := flag.NewFlagSet("upgrade "+args[0], flag.ContinueOnError)
	o := lifecycle.Options{}
	fs.StringVar(&o.Root, "root", "", "owned installation root")
	fs.StringVar(&o.Label, "service-label", "", "owned launchd label")
	packagePath := fs.String("package", "", "checksummed local candidate ZIP")
	data := fs.String("data-dir", "", "private staged state")
	_ = fs.Bool("json", false, "structured output")
	if e := fs.Parse(args[1:]); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected upgrade arguments")
	}
	switch args[0] {
	case "inspect":
		if *packagePath == "" {
			return fmt.Errorf("--package is required")
		}
		v, e := lifecycle.InspectUpgrade(*packagePath)
		if e != nil {
			return e
		}
		return writeJSON(v)
	case "apply":
		if *packagePath == "" {
			return fmt.Errorf("--package is required")
		}
		v, e := lifecycle.ApplyUpgrade(ctx, *packagePath, o)
		if e != nil {
			return e
		}
		return writeJSON(v)
	case "recover":
		v, e := lifecycle.RecoverUpgrade(ctx, o)
		if e != nil {
			return e
		}
		return writeJSON(v)
	case "validate-state":
		if *data == "" {
			return fmt.Errorf("private staged --data-dir is required")
		}
		abs, e := filepath.Abs(*data)
		if e != nil {
			return e
		}
		if e = lifecycle.ValidateStagedUpgradeData(abs); e != nil {
			return e
		}
		ownedData := filepath.Join(filepath.Dir(filepath.Dir(abs)), "data")
		if e = webui.ValidateUpgradeConfiguration(abs, ownedData); e != nil {
			return e
		}
		state, e := store.MigrateUpgradeState(ctx, abs)
		if e != nil {
			return e
		}
		instance, e := store.ReadExistingInstance(abs)
		if e != nil {
			return e
		}
		var revision string
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, v := range info.Settings {
				if v.Key == "vcs.revision" {
					revision = v.Value
				}
			}
		}
		return writeJSON(map[string]any{"update_protocol": 1, "source_commit": revision, "instance_id": instance.ID, "state": state})
	default:
		return fmt.Errorf("unknown upgrade action")
	}
}
func execInstalledDaemon(o lifecycle.Options, health bool) error {
	binary := filepath.Join(o.Root, "current", "bin", "aios")
	args := []string{binary, "daemon", "run", "--root", o.Root, "--data-dir", o.DataDir, "--service-label", o.Label, "--launchd"}
	if health {
		args = append(args, "--upgrade-health")
	}
	return replaceProcess(binary, args, os.Environ())
}
