package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func canonicalTemp(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestRejectSymlinkBeforeMutationAndExclusiveLock(t *testing.T) {
	root := canonicalTemp(t)
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if PrepareDir(filepath.Join(link, "unexpected")) == nil {
		t.Fatal("symlink accepted")
	}
	if _, e := os.Stat(filepath.Join(outside, "unexpected")); !os.IsNotExist(e) {
		t.Fatal("symlink target mutated")
	}
	data := filepath.Join(root, "data")
	lock, e := Lock(data)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	if next, e := Lock(data); e == nil {
		next.Close()
		t.Fatal("second daemon admitted")
	}
}

func TestControlFreshLinksStrictPayloadAndRestart(t *testing.T) {
	root := canonicalTemp(t)
	o := Options{Root: root, DataDir: filepath.Join(root, "data")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e := ServeControl(ctx, o, func() State { return State{Running: true, InstanceID: "stable"} }, func() string { return "http://127.0.0.1:9999/#token=one-use" }); e != nil {
		t.Fatal(e)
	}
	state, e := Control(o, "status")
	if e != nil || state.InstanceID != "stable" || state.URL != "" {
		t.Fatalf("status %+v %v", state, e)
	}
	state, e = Control(o, "open")
	if e != nil || !strings.Contains(state.URL, "#token=") {
		t.Fatalf("open %+v %v", state, e)
	}
	controlPath, e := ControlPath(o.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(controlPath)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("insecure control socket")
	}
	for _, payload := range []string{`{"action":"status","extra":true}`, `{"action":"status"}{}`, `{"action":"unknown"}`, strings.Repeat("x", 2048), `{"action":"status"}` + strings.Repeat(" ", 1025) + `{}`} {
		c, e := net.Dial("unix", controlPath)
		if e != nil {
			t.Fatal(e)
		}
		c.SetDeadline(time.Now().Add(2 * time.Second))
		fmt.Fprint(c, payload)
		c.(*net.UnixConn).CloseWrite()
		var result State
		e = json.NewDecoder(c).Decode(&result)
		c.Close()
		if e == nil {
			t.Fatal("invalid control payload accepted")
		}
	}
	if e := ServeControl(ctx, o, func() State { return State{} }, func() string { return "" }); e == nil {
		t.Fatal("live control socket replaced")
	}
}

func TestPlanAndForeignServiceOwnership(t *testing.T) {
	root := canonicalTemp(t)
	o, e := Defaults(Options{home: root, Root: filepath.Join(root, "owned & instance"), Binary: "/private/aios", Label: "dev.aios.completion.fixture"})
	if e != nil {
		t.Fatal(e)
	}
	p, e := MakePlan(o)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(p.Plist, "&amp;") || strings.Contains(p.Plist, "UserName") || !p.RunAtLoad {
		t.Fatal("invalid per-user escaped plan")
	}
	if e = os.MkdirAll(filepath.Dir(p.PlistPath), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p.PlistPath, []byte(p.Plist), 0600); e != nil {
		t.Fatal(e)
	}
	if e = checkServiceFile(o, p); e != nil {
		t.Fatal(e)
	}
	foreign := o
	foreign.Root = filepath.Join(root, "foreign")
	foreign.DataDir = filepath.Join(foreign.Root, "data")
	fp, _ := MakePlan(foreign)
	if checkServiceFile(foreign, fp) == nil {
		t.Fatal("foreign-root service file accepted")
	}
	output := fmt.Sprintf("path = %s\nprogram = %s\narguments = {\n%s\n}\n", p.PlistPath, o.Binary, strings.Join([]string{o.Binary, "daemon", "run", "--root", o.Root, "--data-dir", o.DataDir, "--service-label", o.Label, "--launchd"}, "\n"))
	if !matchesLoadedService([]byte(output), o, p) || matchesLoadedService([]byte(output), foreign, fp) {
		t.Fatal("loaded foreign-root ownership accepted")
	}
	if _, e = os.Stat(foreign.Root); !os.IsNotExist(e) {
		t.Fatal("foreign root mutated")
	}
}

func testArchive(t *testing.T, mutation string) string {
	t.Helper()
	filename := filepath.Join(canonicalTemp(t), "candidate.zip")
	f, e := os.Create(filename)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	files := map[string]string{}
	for _, name := range []string{"bin/aios", "install.sh", "uninstall.sh", "update.sh", "LICENSES.txt", "USER-GUIDE.md", "RELEASE-NOTES.md"} {
		files["candidate/"+name] = "fixture " + name
	}
	m := Manifest{Schema: 1, Version: "1.0.0-test", SourceCommit: strings.Repeat("a", 40), Platform: "darwin", Architecture: "arm64", DiskSchema: 1, CompatibleFrom: []int{1}, SHA256: map[string]string{}}
	for name, contents := range files {
		h := sha256.Sum256([]byte(contents))
		m.SHA256[name] = hex.EncodeToString(h[:])
	}
	switch mutation {
	case "checksum":
		m.SHA256["candidate/bin/aios"] = strings.Repeat("b", 64)
	case "empty":
		m.SHA256 = map[string]string{}
	case "unknown":
		m.SHA256["candidate/unknown"] = strings.Repeat("a", 64)
	case "traversal":
		files["candidate/../../outside"] = "bad"
	case "duplicate":
		w, _ := z.Create("candidate/bin/aios")
		w.Write([]byte("bad"))
	case "symlink":
		h := &zip.FileHeader{Name: "candidate/link"}
		h.SetMode(os.ModeSymlink | 0700)
		w, _ := z.CreateHeader(h)
		w.Write([]byte("/outside"))
	}
	for name, contents := range files {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(contents)); e != nil {
			t.Fatal(e)
		}
	}
	b, _ := json.Marshal(m)
	w, _ := z.Create("candidate/manifest.json")
	w.Write(b)
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	f.Close()
	return filename
}

func TestArchiveRejectsIncompleteInventoriesTamperAndUnsafeMembers(t *testing.T) {
	if m, p, e := VerifyArchive(testArchive(t, "")); e != nil || m.SourceCommit != strings.Repeat("a", 40) || p != "candidate/" {
		t.Fatalf("valid archive rejected %v", e)
	}
	for _, kind := range []string{"checksum", "empty", "unknown", "traversal", "duplicate", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			if _, _, e := VerifyArchive(testArchive(t, kind)); e == nil {
				t.Fatal("unsafe candidate accepted")
			}
		})
	}
}

func TestMetadataSymlinkRejectedWithoutReads(t *testing.T) {
	root := canonicalTemp(t)
	outside := filepath.Join(root, "outside.json")
	os.WriteFile(outside, []byte(`{}`), 0600)
	ownedRoot := filepath.Join(root, "install")
	os.Mkdir(ownedRoot, 0700)
	os.Symlink(outside, filepath.Join(ownedRoot, "installation.json"))
	if _, e := readInstallation(ownedRoot); e == nil {
		t.Fatal("symlink metadata accepted")
	}
}

func TestArchiveDescriptorSurvivesReplacementAndRejectsOverflow(t *testing.T) {
	path := testArchive(t, "")
	reader, e := zip.OpenReader(path)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	replacement := testArchive(t, "traversal")
	if e = os.Rename(replacement, path); e != nil {
		t.Fatal(e)
	}
	if _, _, e = verifyReader(reader); e != nil {
		t.Fatal("verified descriptor changed with pathname replacement")
	}
	if _, _, e = VerifyArchive(path); e == nil {
		t.Fatal("replacement candidate accepted")
	}
	huge := &zip.ReadCloser{Reader: zip.Reader{File: []*zip.File{{FileHeader: zip.FileHeader{Name: "candidate/huge", UncompressedSize64: ^uint64(0)}}}}}
	if _, _, e = verifyReader(huge); e == nil {
		t.Fatal("overflow-sized archive accepted")
	}
}

func TestLaunchAgentsSymlinkRejectedBeforeServiceMutation(t *testing.T) {
	root := canonicalTemp(t)
	outside := filepath.Join(root, "outside")
	os.Mkdir(outside, 0700)
	link := filepath.Join(root, "LaunchAgents")
	os.Symlink(outside, link)
	if serviceDir(link, true) == nil {
		t.Fatal("symlink service directory accepted")
	}
	if serviceDir(filepath.Join(link, "nested"), true) == nil {
		t.Fatal("symlink service ancestor accepted")
	}
	entries, e := os.ReadDir(outside)
	if e != nil || len(entries) != 0 {
		t.Fatal("service target mutated")
	}
	valid := filepath.Join(root, "valid")
	os.Mkdir(valid, 0755)
	os.Chmod(valid, 0755)
	if e = serviceDir(valid, true); e != nil {
		t.Fatal("normal shared-read service directory rejected")
	}
	info, _ := os.Stat(valid)
	if info.Mode().Perm() != 0755 {
		t.Fatal("existing service directory chmodded")
	}
}

func TestLongDataRootUsesOwnedShortSocketWithoutAliases(t *testing.T) {
	root := canonicalTemp(t)
	data := filepath.Join(root, strings.Repeat("long-data-", 15), "data")
	path, e := ControlPath(data)
	if e != nil || len(path) > 100 || strings.HasPrefix(path, data) {
		t.Fatalf("unbounded control path %q %v", path, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o := Options{Root: root, DataDir: data, Label: "dev.aios.completion.long-root"}
	if e = ServeControl(ctx, o, func() State { return State{Running: true, InstanceID: "stable-long-root"} }, func() string { return "http://127.0.0.1:1/#token=fixture" }); e != nil {
		t.Fatal(e)
	}
	s, e := Control(o, "status")
	if e != nil || s.InstanceID != "stable-long-root" {
		t.Fatalf("long root control failed %v", e)
	}
	info, e := os.Stat(filepath.Dir(path))
	if e != nil || info.Mode().Perm() != 0700 || !owned(info) {
		t.Fatal("short socket root is not owner-only")
	}
	other, _ := ControlPath(data + "-other")
	if other == path {
		t.Fatal("data roots collide")
	}
	if e = os.Chmod(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if _, e = Control(o, "status"); e == nil {
		t.Fatal("shared fallback control parent trusted")
	}
	os.Chmod(filepath.Dir(path), 0700)
	fake := filepath.Join(root, "fake-control")
	os.Symlink(filepath.Dir(path), fake)
	if validateControlPath(filepath.Join(fake, "control.sock")) == nil {
		t.Fatal("symlink control parent trusted")
	}
	absent := data + "-unconfigured"
	missing, _ := ControlPath(absent)
	if _, e = Control(Options{Root: root, DataDir: absent}, "status"); e == nil {
		t.Fatal("missing control accepted")
	}
	if _, e = os.Stat(filepath.Dir(missing)); !os.IsNotExist(e) {
		t.Fatal("status created a control directory")
	}
	if cleanupControl(data) == nil {
		t.Fatal("cleanup removed running control")
	}
}
