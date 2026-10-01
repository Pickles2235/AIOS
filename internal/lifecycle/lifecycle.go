// Package lifecycle owns the per-user launchd service, never source workspaces.
package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const DefaultLabel = "dev.aios.daemon"

var labelPattern = regexp.MustCompile(`^dev\.aios\.[a-zA-Z0-9.-]{1,100}$`)

type Options struct {
	Root, DataDir, Binary, Label string
	Managed                      bool
}
type Plan struct {
	Scope        string `json:"scope"`
	RunAtLoad    bool   `json:"run_at_load"`
	Plist        string `json:"plist"`
	PlistPath    string `json:"plist_path"`
	ServiceLabel string `json:"service_label"`
}
type State struct {
	Running         bool   `json:"running"`
	BrowserRequired bool   `json:"browser_required"`
	InstanceID      string `json:"instance_id,omitempty"`
	URL             string `json:"url,omitempty"`
	PID             int    `json:"pid,omitempty"`
	Restart         string `json:"restart"`
}

func Defaults(o Options) (Options, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return o, err
	}
	if o.Root == "" {
		if o.DataDir != "" {
			o.Root = filepath.Dir(o.DataDir)
		} else {
			o.Root = filepath.Join(home, "Library", "Application Support", "AgentOS")
		}
	}
	o.Root, err = filepath.Abs(o.Root)
	if err != nil {
		return o, err
	}
	if o.DataDir == "" {
		o.DataDir = filepath.Join(o.Root, "data")
	}
	o.DataDir, err = filepath.Abs(o.DataDir)
	if err != nil {
		return o, err
	}
	if o.Label == "" {
		if installed, e := readInstallation(o.Root); e == nil {
			o.Label = installed.ServiceLabel
		} else {
			o.Label = DefaultLabel
		}
	}
	if !labelPattern.MatchString(o.Label) {
		return o, fmt.Errorf("invalid AgentOS service label")
	}
	if o.Binary == "" {
		if installed, e := readInstallation(o.Root); e == nil && installed.Installed {
			o.Binary = installed.Binary
		} else {
			o.Binary, err = os.Executable()
		}
		if err != nil {
			return o, err
		}
	}
	o.Binary, err = filepath.Abs(o.Binary)
	return o, err
}

func escaped(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func MakePlan(o Options) (Plan, error) {
	o, err := Defaults(o)
	if err != nil {
		return Plan{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Plan{}, err
	}
	args := []string{o.Binary, "daemon", "run", "--root", o.Root, "--data-dir", o.DataDir, "--service-label", o.Label, "--launchd"}
	var a strings.Builder
	for _, v := range args {
		a.WriteString("<string>" + escaped(v) + "</string>")
	}
	p := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>` + escaped(o.Label) + `</string><key>ProgramArguments</key><array>` + a.String() + `</array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ThrottleInterval</key><integer>10</integer><key>Umask</key><integer>63</integer><key>EnvironmentVariables</key><dict><key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string></dict><key>StandardOutPath</key><string>/dev/null</string><key>StandardErrorPath</key><string>/dev/null</string></dict></plist>`
	return Plan{Scope: "user", RunAtLoad: true, Plist: p, PlistPath: filepath.Join(home, "Library", "LaunchAgents", o.Label+".plist"), ServiceLabel: o.Label}, nil
}

func RequireNative() error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("launchd installation requires macOS Apple Silicon")
	}
	if os.Geteuid() == 0 {
		return fmt.Errorf("run as the login user, without sudo")
	}
	return nil
}
func target(o Options) string { return "gui/" + strconv.Itoa(os.Getuid()) + "/" + o.Label }
func launchctl(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "/bin/launchctl", args...).Run(); err != nil {
		return fmt.Errorf("launchctl %s failed; run in your macOS login session: %w", args[0], err)
	}
	return nil
}

// PrepareDir refuses symlinks and shared permissions before any owned mutations.
func PrepareDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	ancestor := abs
	for {
		_, e := os.Lstat(ancestor)
		if e == nil {
			break
		}
		if !os.IsNotExist(e) {
			return e
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return fmt.Errorf("owned directory has no existing ancestor")
		}
		ancestor = parent
	}
	real, e := filepath.EvalSymlinks(ancestor)
	if e != nil || real != ancestor {
		return fmt.Errorf("owned directory ancestor must not contain symlinks")
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil || resolved != abs {
		return fmt.Errorf("owned directory must be canonical without symlinks")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !owned(info) {
		return fmt.Errorf("owned directory must belong to the login user")
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("owned directory must have owner-only permissions")
	}
	return nil
}

// Service directories may be readable by others (normal macOS mode 0755),
// but must be owned, not writable by others, and never reached through symlinks.
func serviceDir(dir string, create bool) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	ancestor := abs
	for {
		_, e := os.Lstat(ancestor)
		if e == nil {
			break
		}
		if !os.IsNotExist(e) {
			return e
		}
		ancestor = filepath.Dir(ancestor)
	}
	real, e := filepath.EvalSymlinks(ancestor)
	if e != nil || real != ancestor {
		return fmt.Errorf("service directory must be canonical without symlinks")
	}
	if !create {
		if _, e = os.Stat(abs); os.IsNotExist(e) {
			return nil
		} else if e != nil {
			return e
		}
	}
	if create {
		if err = os.MkdirAll(abs, 0700); err != nil {
			return err
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() || !owned(info) || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("service directory must be owned and not writable by others")
	}
	return nil
}

func Start(ctx context.Context, o Options) (State, error) {
	if err := RequireNative(); err != nil {
		return State{}, err
	}
	o, err := Defaults(o)
	if err != nil {
		return State{}, err
	}
	if s, e := Control(o, "status"); e == nil && s.Running {
		return s, nil
	}
	p, err := MakePlan(o)
	if err != nil {
		return State{}, err
	}
	if err = checkServiceFile(o, p); err != nil {
		return State{}, err
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	loaded, loadErr := exec.CommandContext(probe, "/bin/launchctl", "print", target(o)).Output()
	cancel()
	if loadErr == nil && !matchesLoadedService(loaded, o, p) {
		return State{}, fmt.Errorf("loaded service label belongs to a different installation")
	}
	if err = PrepareDir(o.Root); err != nil {
		return State{}, err
	}
	if err = serviceDir(filepath.Dir(p.PlistPath), true); err != nil {
		return State{}, err
	}
	if err = os.WriteFile(p.PlistPath, []byte(p.Plist), 0600); err != nil {
		return State{}, err
	}
	if err = launchctl(ctx, "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), p.PlistPath); err != nil {
		if e := checkLoadedService(ctx, o, p); e != nil {
			return State{}, e
		}
		if e := launchctl(ctx, "kickstart", "-k", target(o)); e != nil {
			return State{}, err
		}
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if s, e := Control(o, "status"); e == nil && s.Running {
			return s, nil
		}
		select {
		case <-ctx.Done():
			return State{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return State{}, fmt.Errorf("daemon did not become healthy; use daemon status and restart in login session")
}

func Stop(ctx context.Context, o Options) error {
	if err := RequireNative(); err != nil {
		return err
	}
	o, err := Defaults(o)
	if err != nil {
		return err
	}
	p, err := MakePlan(o)
	if err != nil {
		return err
	}
	if err = checkServiceFile(o, p); err != nil {
		return err
	}
	// Inspect existence first: a stopped/unregistered disposable service is idempotent.
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if exec.CommandContext(probe, "/bin/launchctl", "print", target(o)).Run() != nil {
		if _, e := Control(o, "status"); e == nil {
			return fmt.Errorf("daemon is running outside launchd; stop its foreground process")
		}
		return nil
	}
	if err = checkLoadedService(ctx, o, p); err != nil {
		return err
	}
	return launchctl(ctx, "bootout", target(o))
}

func checkServiceFile(o Options, p Plan) error {
	if err := serviceDir(filepath.Dir(p.PlistPath), false); err != nil {
		return err
	}
	if info, e := os.Lstat(p.PlistPath); e == nil {
		if !info.Mode().IsRegular() || !owned(info) || info.Mode().Perm() != 0600 {
			return fmt.Errorf("service plist must be a regular file")
		}
		b, e := os.ReadFile(p.PlistPath)
		if e != nil || string(b) != p.Plist {
			return fmt.Errorf("service label belongs to a different installation")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	return nil
}

func matchesLoadedService(output []byte, o Options, p Plan) bool {
	lines := strings.Split(string(output), "\n")
	args := []string{}
	inArgs := false
	program, plist := "", ""
	for _, line := range lines {
		v := strings.TrimSpace(line)
		if strings.HasPrefix(v, "program = ") {
			program = strings.TrimPrefix(v, "program = ")
		}
		if strings.HasPrefix(v, "path = ") {
			plist = strings.TrimPrefix(v, "path = ")
		}
		if v == "arguments = {" {
			inArgs = true
			continue
		}
		if inArgs {
			if v == "}" {
				inArgs = false
			} else {
				args = append(args, v)
			}
		}
	}
	expected := []string{o.Binary, "daemon", "run", "--root", o.Root, "--data-dir", o.DataDir, "--service-label", o.Label, "--launchd"}
	return program == o.Binary && plist == p.PlistPath && strings.Join(args, "\x00") == strings.Join(expected, "\x00")
}

func checkLoadedService(ctx context.Context, o Options, p Plan) error {
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probe, "/bin/launchctl", "print", target(o)).Output()
	if err != nil {
		return err
	}
	if !matchesLoadedService(output, o, p) {
		return fmt.Errorf("loaded service label belongs to a different installation")
	}
	return nil
}

func Control(o Options, action string) (State, error) {
	o, err := Defaults(o)
	if err != nil {
		return State{}, err
	}
	if action != "status" && action != "open" {
		return State{}, fmt.Errorf("invalid control action")
	}
	conn, err := net.DialTimeout("unix", filepath.Join(o.DataDir, "daemon.sock"), time.Second)
	if err != nil {
		return State{Restart: "aios daemon start"}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err = json.NewEncoder(conn).Encode(map[string]string{"action": action}); err != nil {
		return State{}, err
	}
	if c, ok := conn.(*net.UnixConn); ok {
		if err = c.CloseWrite(); err != nil {
			return State{}, err
		}
	}
	var s State
	err = json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&s)
	return s, err
}

// ServeControl uses filesystem access as the local control authority. Capability
// launch links travel only over this owner-only socket; never a public HTTP API.
func ServeControl(ctx context.Context, o Options, status func() State, open func() string) error {
	o, err := Defaults(o)
	if err != nil {
		return err
	}
	if err = PrepareDir(o.DataDir); err != nil {
		return err
	}
	path := filepath.Join(o.DataDir, "daemon.sock")
	if info, e := os.Lstat(path); e == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("control path is not an owned socket")
		}
		if c, e := net.DialTimeout("unix", path, time.Second); e == nil {
			c.Close()
			return fmt.Errorf("daemon already running")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err = os.Chmod(path, 0600); err != nil {
		listener.Close()
		return err
	}
	go func() { <-ctx.Done(); listener.Close() }()
	go func() {
		defer listener.Close()
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(time.Second))
				var in struct {
					Action string `json:"action"`
				}
				payload, e := io.ReadAll(io.LimitReader(conn, 1025))
				if e != nil || len(payload) > 1024 {
					return
				}
				decoder := json.NewDecoder(bytes.NewReader(payload))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&in) != nil {
					return
				}
				if decoder.Decode(&struct{}{}) != io.EOF {
					return
				}
				s := status()
				if in.Action == "open" {
					s.URL = open()
				} else if in.Action != "status" {
					return
				}
				_ = json.NewEncoder(conn).Encode(s)
			}()
		}
	}()
	return nil
}
