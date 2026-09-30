package model

import "time"

const (
	ExtractorVersion         = "tree-sitter-ts-0.23.2-js-0.25.0-java-0.23.5-kotlin-1.1.0+compiler-v1+domain-v1"
	SchemaVersion            = 3
	SourceKindRepository     = "repository"
	RepositoryAdapterVersion = "git-v1"
)

// AssertionStatus says how strongly a returned record is supported.  It is
// deliberately separate from confidence: neither a projection score nor a
// confidence value is source evidence.
type AssertionStatus string

const (
	AssertionDirect      AssertionStatus = "direct"
	AssertionDerived     AssertionStatus = "derived"
	AssertionInferred    AssertionStatus = "inferred"
	AssertionStale       AssertionStatus = "stale"
	AssertionBounded     AssertionStatus = "bounded"
	AssertionUnavailable AssertionStatus = "unavailable"
)

type DiagnosticSeverity string

const (
	DiagnosticInfo    DiagnosticSeverity = "info"
	DiagnosticWarning DiagnosticSeverity = "warning"
	DiagnosticError   DiagnosticSeverity = "error"
)

// DiagnosticScope is intentionally source-content-free.  It may identify a
// canonical record and source location, but never carries request values,
// excerpts, credentials, or arbitrary error text.
type DiagnosticScope struct {
	Repository string `json:"repository,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Generation string `json:"generation,omitempty"`
	Path       string `json:"path,omitempty"`
	Projection string `json:"projection,omitempty"`
	Query      string `json:"query,omitempty"`
	Handle     string `json:"handle,omitempty"`
}

type DiagnosticEvent struct {
	ID, Code, Timestamp, ResolvedAt, Resolution string
	Severity                                    DiagnosticSeverity
	Scope                                       DiagnosticScope
	Remediation                                 string
	Metadata                                    map[string]string
}

type Repository struct {
	ID        string      `json:"id"`
	Root      string      `json:"root"`
	Include   []string    `json:"include,omitempty"`
	Exclude   []string    `json:"exclude,omitempty"`
	Ownership []Ownership `json:"ownership,omitempty"`
}

// SourceIdentity is the stable provenance boundary compiled into canonical IR.
// V1 supports only repository sources, materialised from approved Git mirrors.
type SourceIdentity struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	AdapterVersion string `json:"adapter_version"`
}

// Ownership is an approved catalog assertion, not an inference. Coordinate is
// either "repository" or an exact module/package coordinate in that repository.
type Ownership struct {
	Coordinate string `json:"coordinate"`
	Owner      string `json:"owner"`
}

// CompilerRuntime contains optional agent-owned executables. Repository-local
// TypeScript remains the first choice; these paths are only a read-only
// fallback and must never point inside a catalogued checkout.
type CompilerRuntime struct {
	JavaHome         string `json:"java_home,omitempty"`
	Node             string `json:"node,omitempty"`
	TypeScriptModule string `json:"typescript_module,omitempty"`
}

type Limits struct {
	MaxFileBytes         int64 `json:"max_file_bytes"`
	MaxFilesPerRepo      int   `json:"max_files_per_repo"`
	MaxTotalBytesPerRepo int64 `json:"max_total_bytes_per_repo"`
	MaxResults           int   `json:"max_results"`
}

type File struct {
	RepoID         string `json:"repo_id"`
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	Size           int64  `json:"size"`
	Language       string `json:"language"`
	Classification string `json:"classification"`
	Content        string `json:"-"`
}

// CoverageReport is immutable accounting for one ingestion run. It records
// every inclusion and exclusion that can affect an absence conclusion.
type CoverageReport struct {
	Entries []CoverageEntry `json:"entries"`
}

type CoverageEntry struct {
	Path           string `json:"path,omitempty"`
	Language       string `json:"language,omitempty"`
	Classification string `json:"classification,omitempty"`
	Outcome        string `json:"outcome"` // included, excluded, unreadable, unsupported
	Reason         string `json:"reason,omitempty"`
	Capability     string `json:"capability,omitempty"`
	Diagnostic     string `json:"diagnostic,omitempty"`
}

// FileChange is a deterministic comparison between two catalogued file
// manifests.  OldPath is populated only for a rename (the content hash is
// unchanged); paths always use slash separators.
type FileChange struct {
	Kind, Path, OldPath string
	SHA256              string
}

type Span struct {
	StartByte   int `json:"start_byte"`
	EndByte     int `json:"end_byte"`
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}

type Symbol struct {
	RepoID     string  `json:"repo_id"`
	Path       string  `json:"path"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Span       Span    `json:"span"`
	Extractor  string  `json:"extractor"`
	Identity   string  `json:"identity,omitempty"`
	Confidence float64 `json:"confidence"`
}

type Edge struct {
	RepoID         string  `json:"repo_id"`
	Path           string  `json:"path"`
	Source         string  `json:"source"`
	Target         string  `json:"target"`
	Kind           string  `json:"kind"`
	Span           Span    `json:"span"`
	Resolver       string  `json:"resolver"`
	SourceIdentity string  `json:"source_identity,omitempty"`
	TargetIdentity string  `json:"target_identity,omitempty"`
	Derivation     string  `json:"derivation,omitempty"`
	Confidence     float64 `json:"confidence"`
}

type GitState struct {
	Commit         string `json:"commit"`
	Branch         string `json:"branch"`
	Dirty          bool   `json:"dirty"`
	UntrackedCount int    `json:"untracked_count"`
}

type Snapshot struct {
	RepoID              string               `json:"repo_id"`
	Source              SourceIdentity       `json:"source"`
	Root                string               `json:"root"`
	Git                 GitState             `json:"git"`
	ContentHash         string               `json:"content_hash"`
	FileCount           int                  `json:"file_count"`
	TotalBytes          int64                `json:"total_bytes"`
	IndexedAt           time.Time            `json:"indexed_at"`
	ExtractorVersions   string               `json:"extractor_versions"`
	CompilerDiagnostics []CompilerDiagnostic `json:"compiler_diagnostics,omitempty"`
}

type CompilerDiagnostic struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

type SearchHit struct {
	EvidenceID string `json:"evidence_id"`
	RepoID     string `json:"repo_id"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Span       Span   `json:"span"`
	Snippet    string `json:"snippet"`
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}
