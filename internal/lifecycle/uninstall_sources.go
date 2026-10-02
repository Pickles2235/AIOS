package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/AdamNi-7080/AIOS/internal/installstate"
	"github.com/AdamNi-7080/AIOS/internal/mirror"
)

func uninstallSourceRoots(data string) ([]string, error) {
	f, e := OpenBoundedMetadata(filepath.Join(data, "setup.json"), 1<<20)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	// Read source authority only. Unrelated setup fields do not grant deletion.
	var setup struct {
		Local []struct {
			Path string `json:"path"`
		} `json:"local_repositories"`
		Mirrors []mirror.Repository `json:"repositories"`
	}
	d := json.NewDecoder(io.LimitReader(f, (1<<20)+1))
	if e = d.Decode(&setup); e != nil {
		return nil, e
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return nil, fmt.Errorf("invalid source approval metadata")
	}
	if len(setup.Local)+len(setup.Mirrors) > 200 {
		return nil, fmt.Errorf("source approval bound exceeded")
	}
	var roots []string
	for _, entry := range setup.Local {
		if !filepath.IsAbs(entry.Path) {
			return nil, fmt.Errorf("invalid approved local path")
		}
		roots = append(roots, entry.Path)
	}
	for _, entry := range setup.Mirrors {
		if filepath.IsAbs(entry.URL) {
			roots = append(roots, entry.URL)
			continue
		}
		u, e := url.Parse(entry.URL)
		if e != nil {
			return nil, e
		}
		if u.Scheme == "file" {
			if !filepath.IsAbs(u.Path) {
				return nil, fmt.Errorf("invalid approved file remote")
			}
			roots = append(roots, u.Path)
		}
	}
	return roots, nil
}

func validateUninstallSourceRoots(o Options, preserve bool, roots []string) error {
	reg := mirror.Registry{Version: 1}
	for _, root := range roots {
		if !filepath.IsAbs(root) || len(root) > 4096 {
			return fmt.Errorf("invalid saved source boundary")
		}
		reg.Repositories = append(reg.Repositories, mirror.Repository{URL: root})
	}
	targets := []string{o.Root}
	if preserve {
		targets = []string{filepath.Join(o.Root, "current"), filepath.Join(o.Root, "recovery")}
	}
	p, e := MakePlan(o)
	if e != nil {
		return e
	}
	targets = append(targets, p.PlistPath, installstate.Authority(o.Root))
	for _, target := range targets {
		if e := mirror.ValidateSourceBoundaries(reg, target); e != nil {
			return fmt.Errorf("uninstall would overlap an approved source; move its source outside the installation before retrying")
		}
	}
	return nil
}

// ValidateInstallationSourceBoundaries protects all installed assets, including
// the launch agent outside the root, when accepting or replaying approvals.
func ValidateInstallationSourceBoundaries(reg mirror.Registry, data string) error {
	if e := mirror.ValidateSourceBoundaries(reg, data); e != nil {
		return e
	}
	if filepath.Base(data) != "data" {
		return nil
	}
	root := filepath.Dir(data)
	v, e := readInstallation(root)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	o := Options{Root: root, DataDir: data, Label: v.ServiceLabel, Binary: v.Binary}
	p, e := uninstallPlan(o, v)
	if e != nil {
		return e
	}
	for _, target := range []string{root, p.PlistPath, installstate.Authority(root)} {
		if e = mirror.ValidateSourceBoundaries(reg, target); e != nil {
			return e
		}
	}
	return nil
}

func validateUpgradeSourceRoots(o Options) error {
	roots, e := uninstallSourceRoots(o.DataDir)
	if e != nil {
		return e
	}
	return validateUninstallSourceRoots(o, false, roots)
}
