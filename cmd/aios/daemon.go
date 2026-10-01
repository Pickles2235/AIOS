package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"github.com/AdamNi-7080/AIOS/internal/webui"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
)

func runDaemon(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("daemon requires plan|run|start|stop|status|open")
	}
	fs := flag.NewFlagSet("daemon "+args[0], flag.ContinueOnError)
	o := lifecycle.Options{}
	fs.StringVar(&o.Root, "root", "", "owned installation root")
	fs.StringVar(&o.DataDir, "data-dir", "", "owned knowledge data directory")
	fs.StringVar(&o.Label, "service-label", "", "per-user launchd service label")
	fs.BoolVar(&o.Managed, "launchd", false, "run as the managed launchd child")
	_ = fs.Bool("json", false, "structured JSON output")
	recovery := fs.Bool("recovery", false, "open/link through localhost recovery without renaming the namespace")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected daemon arguments")
	}
	var err error
	o, err = lifecycle.Defaults(o)
	if err != nil {
		return err
	}
	switch args[0] {
	case "plan":
		p, e := lifecycle.MakePlan(o)
		if e != nil {
			return e
		}
		return writeJSON(p)
	case "start":
		s, e := lifecycle.Start(ctx, o)
		if e != nil {
			return e
		}
		return writeJSON(s)
	case "stop":
		if e := lifecycle.Stop(ctx, o); e != nil {
			return e
		}
		return writeJSON(map[string]any{"running": false, "restart": "aios daemon start"})
	case "status":
		s, e := lifecycle.Control(o, "status")
		if e != nil {
			return writeJSON(map[string]any{"running": false, "restart": "aios daemon start", "service_state": "unreachable"})
		}
		return writeJSON(s)
	case "credentials":
		s, e := lifecycle.Control(o, "credentials")
		if e != nil {
			return fmt.Errorf("daemon unavailable; start a disposable or installed daemon to inspect its actual credential context")
		}
		return writeJSON(s.Credentials)
	case "open":
		s, e := lifecycle.Control(o, "open")
		if e != nil {
			return fmt.Errorf("daemon unavailable; run aios daemon start: %w", e)
		}
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("native browser open requires macOS; use daemon run for developer smoke")
		}
		if *recovery {
			u, e := url.Parse(s.URL)
			if e != nil {
				return e
			}
			u.Host = net.JoinHostPort("localhost", u.Port())
			s.URL = u.String()
		}
		if e = exec.CommandContext(ctx, "/usr/bin/open", s.URL).Run(); e != nil {
			return e
		}
		s.URL = ""
		return writeJSON(s)
	case "link":
		s, e := lifecycle.Control(o, "open")
		if e != nil {
			return fmt.Errorf("daemon unavailable; start it before requesting a private launch link")
		}
		if *recovery {
			u, e := url.Parse(s.URL)
			if e != nil {
				return e
			}
			u.Host = net.JoinHostPort("localhost", u.Port())
			s.URL = u.String()
		}
		return writeJSON(s)
	case "run":
		return serveDaemon(ctx, o)
	default:
		return fmt.Errorf("unknown daemon action")
	}
}

func serveDaemon(ctx context.Context, o lifecycle.Options) error {
	lock, err := lifecycle.Lock(o.DataDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	w, err := store.OpenWriter(o.DataDir)
	if err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	db, err := store.OpenReadOnly(o.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	instance, err := store.LoadInstance(o.DataDir)
	if err != nil {
		return err
	}
	s, err := webui.New(catalog.Config{Version: 1, Limits: catalog.Defaults()}, db, nil)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if runtime.GOOS == "darwin" && o.Managed {
		s.SetStopDaemon(func() error {
			if e := lifecycle.Stop(context.Background(), o); e != nil {
				return e
			}
			cancel()
			return nil
		})
	} else {
		s.SetStopDaemon(func() error { cancel(); return nil })
	}
	credentials := func() map[string]bool {
		return mirror.CredentialStatus(ctx, runtime.GOOS == "darwin" && o.Managed && os.Getppid() == 1)
	}
	if err = lifecycle.ServeControlWithCredentials(ctx, o, func() lifecycle.State {
		return lifecycle.State{Running: true, InstanceID: instance.ID, PID: os.Getpid(), Restart: "aios daemon start; aios daemon open"}
	}, s.FreshURL, credentials); err != nil {
		return err
	}
	// Native launchd discards stderr. Developer foreground launch links remain
	// one-use capabilities and are never written to persistent logs.
	if !o.Managed {
		fmt.Fprintln(os.Stderr, "Open local UI:", s.URL())
	}
	return s.Serve(ctx)
}

func runInstallation(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	o := lifecycle.Options{}
	fs.StringVar(&o.Root, "root", "", "owned installation root")
	fs.StringVar(&o.Label, "service-label", "", "disposable label for acceptance, default single user instance")
	packagePath := fs.String("package", "", "checksummed candidate ZIP")
	preserve := fs.Bool("preserve-data", false, "preserve owned onboarding/configuration/KB")
	remove := fs.Bool("delete-data", false, "delete all owned data")
	_ = fs.Bool("json", false, "structured output")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected installation arguments")
	}
	if args[0] == "install" {
		if *packagePath == "" {
			return fmt.Errorf("--package is required")
		}
		v, e := lifecycle.Install(ctx, *packagePath, o)
		if e != nil {
			return e
		}
		return writeJSON(v)
	}
	if *preserve == *remove {
		return fmt.Errorf("choose exactly one of --preserve-data or --delete-data")
	}
	if err := lifecycle.Uninstall(ctx, o, *preserve); err != nil {
		return err
	}
	return writeJSON(map[string]any{"uninstalled": true, "data_preserved": *preserve})
}
