package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/installstate"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
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
	CompatibleIR      []string          `json:"compatible_ir_formats,omitempty"`
	UpdateProtocol    int               `json:"update_protocol,omitempty"`
	SHA256            map[string]string `json:"sha256"`
}
type Installation = installstate.Installation

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
	if err = os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	return syncUpgradeDir(dir)
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
	if err = validateGitEnvironment(v.GitEnvironment); err != nil {
		return v, err
	}
	return v, nil
}

func Install(ctx context.Context, filename string, o Options) (Installation, error) {
	if err := RequireNative(); err != nil {
		return Installation{}, err
	}
	return installCandidate(ctx, filename, o, nativeUpgradeOperations())
}

func Uninstall(ctx context.Context, o Options, preserve bool) error {
	if err := RequireNative(); err != nil {
		return err
	}
	return uninstallCandidate(ctx, o, preserve, stopDaemon)
}
