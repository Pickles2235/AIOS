package webui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/localname"
)

func readUpgradeControl(data, name string, limit int64, value any) (bool, error) {
	f, e := lifecycle.OpenBoundedMetadata(filepath.Join(data, name), limit)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, limit+1))
	d.DisallowUnknownFields()
	if e = d.Decode(value); e != nil {
		return true, e
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return true, fmt.Errorf("invalid saved update control")
	}
	return true, nil
}

// ValidateUpgradeConfiguration checks startup controls without restoring them,
// opening namespace services, repairing files, or contacting source remotes.
// A pending purge must finish under the prior daemon before a state-preserving
// upgrade; its startup side effects cannot be part of read-only health.
func ValidateUpgradeConfiguration(data, ownedData string) error {
	var setup Setup
	exists, e := readUpgradeControl(data, "setup.json", 1<<20, &setup)
	if e != nil {
		return e
	}
	if exists {
		if _, _, e = managementConfig(setup); e != nil {
			return e
		}
		for _, root := range []string{data, ownedData} {
			if e = lifecycle.ValidateInstallationSourceBoundaries(approvedAssetSources(setup), root); e != nil {
				return e
			}
		}
	}
	var namespace namespaceConfig
	if exists, e = readUpgradeControl(data, "namespace.json", 4096, &namespace); e != nil {
		return e
	} else if exists {
		if e = localname.Validate(namespace.Name); e != nil {
			return e
		}
	}
	var removal removalIntent
	if exists, e = readUpgradeControl(data, removalJournal, 1024, &removal); e != nil {
		return e
	} else if exists {
		return fmt.Errorf("finish pending repository removal with the installed daemon before updating")
	}
	return nil
}
