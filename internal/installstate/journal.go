package installstate

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var transactionPattern = regexp.MustCompile(`^\.upgrade-[a-f0-9]{24}$`)
var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var labelPattern = regexp.MustCompile(`^dev\.aios\.[a-zA-Z0-9.-]{1,100}$`)
var repositoryAssetID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
var gitPathKeys = map[string]bool{"GIT_CONFIG_GLOBAL": true, "GIT_SSL_CAINFO": true, "SSL_CERT_FILE": true}

type Installation struct {
	Scope            string            `json:"scope"`
	Binary           string            `json:"binary"`
	DataDir          string            `json:"data_dir"`
	ServiceLabel     string            `json:"service_label"`
	Version          string            `json:"version"`
	Installed        bool              `json:"installed"`
	GitEnvironment   map[string]string `json:"git_environment,omitempty"`
	RecoveryLauncher bool              `json:"recovery_launcher,omitempty"`
}

type UpgradeState struct {
	Format               string            `json:"format"`
	CanonicalFingerprint string            `json:"canonical_fingerprint"`
	ActiveGenerations    map[string]string `json:"active_generations"`
	RebuiltProjections   []string          `json:"rebuilt_projections"`
}

type UpgradeJournal struct {
	Schema           int          `json:"schema_version"`
	Transaction      string       `json:"transaction"`
	Phase            string       `json:"phase"`
	Previous         Installation `json:"previous"`
	PreviousBinary   string       `json:"previous_binary_sha256"`
	CandidateBinary  string       `json:"candidate_binary_sha256"`
	RecoveryBinary   string       `json:"recovery_binary_sha256,omitempty"`
	RecoveryRequired bool         `json:"recovery_required,omitempty"`
	PreviousState    UpgradeState `json:"previous_state"`
	SnapshotCaptured bool         `json:"snapshot_captured"`
	InstanceID       string       `json:"instance_id"`
	WasRunning       bool         `json:"was_running"`
	Version          string       `json:"version"`
}

type InstallJournal struct {
	Schema             int           `json:"schema_version"`
	Transaction        string        `json:"transaction"`
	Phase              string        `json:"phase"`
	Installation       Installation  `json:"installation"`
	Previous           *Installation `json:"previous,omitempty"`
	HadData            bool          `json:"had_data"`
	Binary             string        `json:"binary_sha256"`
	InstanceID         string        `json:"instance_id"`
	State              UpgradeState  `json:"state"`
	OriginalState      UpgradeState  `json:"original_state,omitempty"`
	OriginalInstanceID string        `json:"original_instance_id,omitempty"`
}

func ValidateUpgradeJournal(root string, j UpgradeJournal) error {
	if j.Schema != 1 || !transactionPattern.MatchString(j.Transaction) || !slices.Contains([]string{"stopping", "preparing", "staged", "migrated", "activation", "health", "committed", "rollback"}, j.Phase) || !shaPattern.MatchString(j.PreviousBinary) || !shaPattern.MatchString(j.CandidateBinary) || j.Previous.Scope != "user" || !j.Previous.Installed || j.Previous.Binary != filepath.Join(root, "current", "bin", "aios") || j.Previous.DataDir != filepath.Join(root, "data") || !labelPattern.MatchString(j.Previous.ServiceLabel) || len(j.InstanceID) != 32 || j.Version == "" {
		return fmt.Errorf("invalid owned upgrade journal")
	}
	if j.SnapshotCaptured {
		if !shaPattern.MatchString(j.PreviousState.CanonicalFingerprint) {
			return fmt.Errorf("invalid upgrade snapshot fingerprint")
		}
	} else if (j.Phase != "stopping" && j.Phase != "rollback") || j.PreviousState.CanonicalFingerprint != "" {
		return fmt.Errorf("missing upgrade snapshot authority")
	}
	if _, e := hex.DecodeString(j.InstanceID); e != nil {
		return fmt.Errorf("invalid upgrade identity")
	}
	if j.RecoveryBinary != "" && !shaPattern.MatchString(j.RecoveryBinary) {
		return fmt.Errorf("invalid recovery checksum")
	}
	if (j.RecoveryRequired || j.Previous.RecoveryLauncher || slices.Contains([]string{"activation", "health", "committed"}, j.Phase)) && (!shaPattern.MatchString(j.RecoveryBinary) || !j.RecoveryRequired) {
		return fmt.Errorf("missing durable recovery authority")
	}
	if e := ValidateGitEnvironment(j.Previous.GitEnvironment); e != nil {
		return e
	}
	if len(j.PreviousState.ActiveGenerations) > 100 {
		return fmt.Errorf("invalid upgrade catalog bound")
	}
	for id, g := range j.PreviousState.ActiveGenerations {
		if !repositoryAssetID.MatchString(id) || len(g) > 128 || g == "" {
			return fmt.Errorf("invalid upgrade generations")
		}
	}
	return nil
}

func ValidateInstallJournal(root string, j InstallJournal) error {
	if j.Schema != 1 || !transactionPattern.MatchString(j.Transaction) || !slices.Contains([]string{"preparing", "activation", "committed", "rollback"}, j.Phase) || !shaPattern.MatchString(j.Binary) || j.Installation.Scope != "user" || j.Installation.Binary != filepath.Join(root, "current", "bin", "aios") || j.Installation.DataDir != filepath.Join(root, "data") || !labelPattern.MatchString(j.Installation.ServiceLabel) || !j.Installation.Installed {
		return fmt.Errorf("invalid initial installation journal")
	}
	if e := ValidateGitEnvironment(j.Installation.GitEnvironment); e != nil {
		return e
	}
	if j.Previous != nil && (j.Previous.Installed || j.Previous.Scope != "user" || j.Previous.Binary != j.Installation.Binary || j.Previous.DataDir != j.Installation.DataDir || j.Previous.ServiceLabel != j.Installation.ServiceLabel || j.Previous.RecoveryLauncher) {
		return fmt.Errorf("invalid preserved installation authority")
	}
	if j.HadData {
		if j.Previous == nil || !shaPattern.MatchString(j.OriginalState.CanonicalFingerprint) || !slices.Contains([]string{"knowledge-ir-v9", "knowledge-ir-v10"}, j.OriginalState.Format) || len(j.OriginalInstanceID) != 32 {
			return fmt.Errorf("invalid preserved data authority")
		}
		if _, e := hex.DecodeString(j.OriginalInstanceID); e != nil {
			return e
		}
	}
	if j.Phase == "activation" || j.Phase == "committed" {
		if len(j.InstanceID) != 32 || !shaPattern.MatchString(j.State.CanonicalFingerprint) || j.State.Format != "knowledge-ir-v10" {
			return fmt.Errorf("invalid staged installation authority")
		}
		if _, e := hex.DecodeString(j.InstanceID); e != nil {
			return e
		}
	}
	return nil
}

func ValidateGitEnvironment(values map[string]string) error {
	for key, value := range values {
		if !gitPathKeys[key] || len(value) > 4096 || !filepath.IsAbs(value) || filepath.Clean(value) != value || strings.ContainsAny(value, "\x00\n\r") {
			return fmt.Errorf("Git environment permits only bounded external configuration paths")
		}
	}
	return nil
}
