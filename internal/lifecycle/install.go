package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type Manifest struct {
	Schema            int               `json:"schema_version"`
	Version           string            `json:"version"`
	SourceCommit      string            `json:"source_commit"`
	Platform          string            `json:"platform"`
	Architecture      string            `json:"architecture"`
	DiskSchema        int               `json:"disk_schema"`
	KnowledgeIRFormat string            `json:"knowledge_ir_format"`
	CompatibleFrom    []int             `json:"compatible_from"`
	SHA256            map[string]string `json:"sha256"`
}
type Installation struct {
	Scope        string `json:"scope"`
	Binary       string `json:"binary"`
	DataDir      string `json:"data_dir"`
	ServiceLabel string `json:"service_label"`
	Version      string `json:"version"`
	Installed    bool   `json:"installed"`
}

var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

// VerifyArchive is portable validation. Extraction never trusts ZIP filenames,
// special files, duplicate entries, partial inventories or declared checksums.
func VerifyArchive(filename string) (Manifest, string, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return Manifest{}, "", err
	}
	defer reader.Close()
	return verifyReader(reader)
}

func verifyReader(reader *zip.ReadCloser) (Manifest, string, error) {
	var err error
	var m Manifest
	var prefix string
	var manifestFile *zip.File
	files := map[string]*zip.File{}
	var total uint64
	if len(reader.File) > 256 {
		return m, "", fmt.Errorf("candidate has too many entries")
	}
	for _, f := range reader.File {
		n := f.Name
		if n == "" || strings.Contains(n, "\\") || strings.HasPrefix(n, "/") || path.Clean(n) != strings.TrimSuffix(n, "/") || strings.Contains(n, "\x00") || strings.HasPrefix(n, "../") {
			return m, "", fmt.Errorf("unsafe archive path")
		}
		if f.Mode().IsDir() {
			continue
		}
		if !f.Mode().IsRegular() {
			return m, "", fmt.Errorf("archive special files are forbidden")
		}
		if files[n] != nil {
			return m, "", fmt.Errorf("duplicate archive file")
		}
		files[n] = f
		if f.UncompressedSize64 > (2<<30)-total {
			return m, "", fmt.Errorf("candidate exceeds 2 GiB")
		}
		total += f.UncompressedSize64
		if strings.HasSuffix(n, "/manifest.json") {
			if manifestFile != nil {
				return m, "", fmt.Errorf("multiple manifests")
			}
			manifestFile = f
			prefix = strings.TrimSuffix(n, "manifest.json")
		}
	}
	if manifestFile == nil || manifestFile.UncompressedSize64 > 65536 {
		return m, "", fmt.Errorf("bounded manifest required")
	}
	r, err := manifestFile.Open()
	if err != nil {
		return m, "", err
	}
	decoder := json.NewDecoder(io.LimitReader(r, 65537))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&m)
	if err == nil && decoder.Decode(&struct{}{}) != io.EOF {
		err = fmt.Errorf("invalid manifest trailing data")
	}
	r.Close()
	if err != nil {
		return m, "", err
	}
	if m.Schema != 1 || !commitPattern.MatchString(m.SourceCommit) || m.Version == "" || m.DiskSchema < 1 || len(m.SHA256) != len(files)-1 || len(m.SHA256) < 7 {
		return m, "", fmt.Errorf("incomplete candidate manifest")
	}
	for _, required := range []string{"bin/aios", "install.sh", "uninstall.sh", "update.sh", "LICENSES.txt", "USER-GUIDE.md", "RELEASE-NOTES.md"} {
		if files[prefix+required] == nil {
			return m, "", fmt.Errorf("candidate missing %s", required)
		}
	}
	for n, f := range files {
		if !strings.HasPrefix(n, prefix) {
			return m, "", fmt.Errorf("candidate has multiple roots")
		}
		if f == manifestFile {
			continue
		}
		expected, ok := m.SHA256[n]
		if !ok || !shaPattern.MatchString(expected) {
			return m, "", fmt.Errorf("missing file checksum")
		}
		r, err := f.Open()
		if err != nil {
			return m, "", err
		}
		h := sha256.New()
		var size int64
		size, err = io.Copy(h, io.LimitReader(r, int64(f.UncompressedSize64)+1))
		r.Close()
		if err != nil {
			return m, "", err
		}
		if size != int64(f.UncompressedSize64) {
			return m, "", fmt.Errorf("candidate size mismatch")
		}
		if hex.EncodeToString(h.Sum(nil)) != expected {
			return m, "", fmt.Errorf("candidate checksum mismatch")
		}
	}
	return m, prefix, nil
}

func writeOwnedJSON(dir, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".metadata-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(dir, name))
}

func readInstallation(root string) (Installation, error) {
	var v Installation
	p := filepath.Join(root, "installation.json")
	f, err := openOwned(p)
	if err != nil {
		return v, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(b, &v); err != nil {
		return v, err
	}
	if v.Scope != "user" || v.DataDir != filepath.Join(root, "data") || v.Binary != filepath.Join(root, "current", "bin", "aios") || !labelPattern.MatchString(v.ServiceLabel) {
		return v, fmt.Errorf("invalid owned installation metadata")
	}
	return v, nil
}

func Install(ctx context.Context, filename string, o Options) (Installation, error) {
	if err := RequireNative(); err != nil {
		return Installation{}, err
	}
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return Installation{}, err
	}
	defer reader.Close()
	m, prefix, err := verifyReader(reader)
	if err != nil {
		return Installation{}, err
	}
	if m.Platform != runtime.GOOS || m.Architecture != runtime.GOARCH {
		return Installation{}, fmt.Errorf("candidate does not match macOS Apple Silicon")
	}
	o, err = Defaults(o)
	if err != nil {
		return Installation{}, err
	}
	if entries, e := os.ReadDir(o.Root); e == nil && len(entries) > 0 {
		old, e := readInstallation(o.Root)
		if e != nil || old.Installed {
			return Installation{}, fmt.Errorf("installation root is occupied; use manual upgrade for an installed candidate")
		}
	} else if e != nil && !os.IsNotExist(e) {
		return Installation{}, e
	}
	if err = PrepareDir(o.Root); err != nil {
		return Installation{}, err
	}
	v := Installation{Scope: "user", Binary: filepath.Join(o.Root, "current", "bin", "aios"), DataDir: filepath.Join(o.Root, "data"), ServiceLabel: o.Label, Version: m.Version}
	if err = writeOwnedJSON(o.Root, "installation.json", v); err != nil {
		return v, err
	}
	stage, err := os.MkdirTemp(o.Root, ".install-stage-*")
	if err != nil {
		return v, err
	}
	defer os.RemoveAll(stage)
	for _, z := range reader.File {
		if err = ctx.Err(); err != nil {
			return v, err
		}
		if z.Mode().IsDir() {
			continue
		}
		rel := strings.TrimPrefix(z.Name, prefix)
		if rel == "manifest.json" {
			if err = writeOwnedJSON(stage, "manifest.json", m); err != nil {
				return v, err
			}
			continue
		}
		dest := filepath.Join(stage, filepath.FromSlash(rel))
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return v, err
		}
		mode := os.FileMode(0600)
		if rel == "bin/aios" || strings.HasSuffix(rel, ".sh") {
			mode = 0700
		}
		out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if e != nil {
			return v, e
		}
		in, e := z.Open()
		if e != nil {
			out.Close()
			return v, e
		}
		h := sha256.New()
		var copied int64
		copied, e = io.Copy(io.MultiWriter(out, h), io.LimitReader(in, int64(z.UncompressedSize64)+1))
		in.Close()
		if e == nil && copied != int64(z.UncompressedSize64) {
			e = fmt.Errorf("candidate extraction size mismatch")
		}
		if e == nil && hex.EncodeToString(h.Sum(nil)) != m.SHA256[z.Name] {
			e = fmt.Errorf("candidate changed during extraction")
		}
		if e == nil {
			e = out.Sync()
		}
		ce := out.Close()
		if e != nil {
			return v, e
		}
		if ce != nil {
			return v, ce
		}
	}
	if err = PrepareDir(v.DataDir); err != nil {
		return v, err
	}
	if err = os.Rename(stage, filepath.Join(o.Root, "current")); err != nil {
		return v, err
	}
	v.Installed = true
	if err = writeOwnedJSON(o.Root, "installation.json", v); err != nil {
		return v, err
	}
	return v, nil
}

func Uninstall(ctx context.Context, o Options, preserve bool) error {
	if err := RequireNative(); err != nil {
		return err
	}
	o, err := Defaults(o)
	if err != nil {
		return err
	}
	if _, err = os.Stat(o.Root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err = PrepareDir(o.Root); err != nil {
		return err
	}
	v, err := readInstallation(o.Root)
	if err != nil {
		return err
	}
	if o.Label != v.ServiceLabel {
		return fmt.Errorf("service label differs from owned installation")
	}
	o.Binary = v.Binary
	p, err := MakePlan(o)
	if err != nil {
		return err
	}
	if err = checkServiceFile(o, p); err != nil {
		return err
	}
	if err = Stop(ctx, o); err != nil {
		return err
	}
	if err = cleanupControl(v.DataDir); err != nil {
		return err
	}
	if b, e := os.ReadFile(p.PlistPath); e == nil {
		if string(b) != p.Plist {
			return fmt.Errorf("service plist belongs to a different installation")
		}
		if e = os.Remove(p.PlistPath); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if !preserve {
		return os.RemoveAll(o.Root)
	}
	if err = os.RemoveAll(filepath.Join(o.Root, "current")); err != nil {
		return err
	}
	v.Installed = false
	return writeOwnedJSON(o.Root, "installation.json", v)
}
