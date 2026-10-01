// Package lifecycle owns the per-user launchd service, never source workspaces.
package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultLabel = "dev.aios.daemon"

var labelPattern = regexp.MustCompile(`^dev\.aios\.[a-zA-Z0-9.-]{1,100}$`)

type Options struct {
	Root, DataDir, Binary, Label string
	Managed                      bool
	home                         string // injected only within this package's ownership tests
	gitEnvironment               map[string]string
}
type Plan struct {
	Scope        string `json:"scope"`
	RunAtLoad    bool   `json:"run_at_load"`
	Plist        string `json:"plist"`
	PlistPath    string `json:"plist_path"`
	ServiceLabel string `json:"service_label"`
}
type State struct {
	Running         bool            `json:"running"`
	BrowserRequired bool            `json:"browser_required"`
	InstanceID      string          `json:"instance_id,omitempty"`
	URL             string          `json:"url,omitempty"`
	PID             int             `json:"pid,omitempty"`
	Credentials     map[string]bool `json:"credentials,omitempty"`
	Restart         string          `json:"restart"`
}

// OpenMetadata verifies local authority before opening shared product metadata.
// Callers additionally enforce their smaller format-specific size limit.
func OpenMetadata(path string) (*os.File, error) { return openOwned(path) }

func Defaults(o Options) (Options, error) {
	home := o.home
	var err error
	if home == "" {
		home, err = os.UserHomeDir()
	}
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
	if installed, e := readInstallation(o.Root); e == nil {
		o.gitEnvironment = installed.GitEnvironment
	}
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
	home := o.home
	if home == "" {
		home, err = os.UserHomeDir()
	}
	if err != nil {
		return Plan{}, err
	}
	args := []string{o.Binary, "daemon", "run", "--root", o.Root, "--data-dir", o.DataDir, "--service-label", o.Label, "--launchd"}
	var a strings.Builder
	for _, v := range args {
		a.WriteString("<string>" + escaped(v) + "</string>")
	}
	var gitEnv strings.Builder
	keys := []string{}
	for key := range o.gitEnvironment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		gitEnv.WriteString("<key>" + escaped(key) + "</key><string>" + escaped(o.gitEnvironment[key]) + "</string>")
	}
	p := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>` + escaped(o.Label) + `</string><key>ProgramArguments</key><array>` + a.String() + `</array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ThrottleInterval</key><integer>10</integer><key>Umask</key><integer>63</integer><key>EnvironmentVariables</key><dict><key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string></dict><key>StandardOutPath</key><string>/dev/null</string><key>StandardErrorPath</key><string>/dev/null</string></dict></plist>`
	// Environment pointers only; credentials and helper responses stay external.
	p = strings.Replace(p, "</string></dict><key>StandardOutPath</key>", "</string>"+gitEnv.String()+"</dict><key>StandardOutPath</key>", 1)
	return Plan{Scope: "user", RunAtLoad: true, Plist: p, PlistPath: filepath.Join(home, "Library", "LaunchAgents", o.Label+".plist"), ServiceLabel: o.Label}, nil
}

var gitPathKeys = map[string]bool{"GIT_CONFIG_GLOBAL": true, "GIT_SSL_CAINFO": true, "SSL_CERT_FILE": true}

func validateGitEnvironment(values map[string]string) error {
	for key, value := range values {
		if !gitPathKeys[key] || len(value) > 4096 || !filepath.IsAbs(value) || filepath.Clean(value) != value || strings.ContainsAny(value, "\x00\n\r") {
			return fmt.Errorf("Git environment permits only bounded external configuration paths")
		}
	}
	return nil
}
func machineGitEnvironment() (map[string]string, error) {
	values := map[string]string{}
	for key := range gitPathKeys {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	return values, validateGitEnvironment(values)
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
	if action != "status" && action != "open" && action != "credentials" {
		return State{}, fmt.Errorf("invalid control action")
	}
	controlPath, err := ControlPath(o.DataDir)
	if err != nil {
		return State{}, err
	}
	if err = validateControlPath(controlPath); err != nil {
		return State{}, err
	}
	conn, err := net.DialTimeout("unix", controlPath, time.Second)
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

// ControlPath respects macOS's104-byte Unix socket path limit without creating
// symlink aliases for knowledge directories. The fallback directory is owner-only.
func ControlPath(data string) (string, error) {
	abs, err := filepath.Abs(data)
	if err != nil {
		return "", err
	}
	p := filepath.Join(abs, "daemon.sock")
	if len(p) <= 100 {
		return p, nil
	}
	base, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(abs))
	short := filepath.Join(base, "aios-control-"+strconv.Itoa(os.Getuid())+"-"+hex.EncodeToString(h[:12]), "control.sock")
	if len(short) > 100 {
		return "", fmt.Errorf("canonical temporary control path exceeds native socket limit")
	}
	return short, nil
}

func validateControlPath(path string) error {
	dir := filepath.Dir(path)
	real, e := filepath.EvalSymlinks(dir)
	if e != nil || real != dir {
		return fmt.Errorf("control parent must be canonical")
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if !info.IsDir() || !owned(info) || info.Mode().Perm() != 0700 {
		return fmt.Errorf("control parent must be owner-only")
	}
	info, e = os.Lstat(path)
	if e != nil {
		return e
	}
	if info.Mode()&os.ModeSocket == 0 || !owned(info) || info.Mode().Perm() != 0600 {
		return fmt.Errorf("control endpoint must be an owned owner-only socket")
	}
	return nil
}

func cleanupControl(data string) error {
	path, err := ControlPath(data)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(path); err == nil {
		if err = validateControlPath(path); err != nil {
			return err
		}
		if conn, e := net.DialTimeout("unix", path, 200*time.Millisecond); e == nil {
			conn.Close()
			return fmt.Errorf("cannot remove running daemon control")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if filepath.Dir(path) != data {
		if err = os.Remove(filepath.Dir(path)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// ServeControl uses filesystem access as the local control authority. Capability
// launch links travel only over this owner-only socket; never a public HTTP API.
func ServeControl(ctx context.Context, o Options, status func() State, open func() string) error {
	return ServeControlWithCredentials(ctx, o, status, open, nil)
}

// Credential diagnostics execute only for an explicit owner control request.
func ServeControlWithCredentials(ctx context.Context, o Options, status func() State, open func() string, credentials func() map[string]bool) error {
	o, err := Defaults(o)
	if err != nil {
		return err
	}
	if err = PrepareDir(o.DataDir); err != nil {
		return err
	}
	path, err := ControlPath(o.DataDir)
	if err != nil {
		return err
	}
	if filepath.Dir(path) != o.DataDir {
		if err = PrepareDir(filepath.Dir(path)); err != nil {
			return err
		}
	}
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
	var once sync.Once
	closeControl := func() {
		once.Do(func() {
			listener.Close()
			if filepath.Dir(path) != o.DataDir {
				_ = os.Remove(filepath.Dir(path))
			}
		})
	}
	go func() { <-ctx.Done(); closeControl() }()
	go func() {
		defer closeControl()
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
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
				} else if in.Action == "credentials" && credentials != nil {
					s.Credentials = credentials()
				} else if in.Action != "status" {
					return
				}
				_ = json.NewEncoder(conn).Encode(s)
			}()
		}
	}()
	return nil
}
