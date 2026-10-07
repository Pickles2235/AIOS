// Package resourcepolicy samples local machine signals and chooses a bounded
// background-work policy. Sampling never reads repository content.
package resourcepolicy

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const MaxWait = 5 * time.Minute
const JournalRetentionBytes int64 = 512 << 10

type Signals struct {
	Provider    string    `json:"signal_provider"`
	Power       string    `json:"power_source"`
	Load        float64   `json:"load"`
	IdleSeconds float64   `json:"idle_seconds"`
	ObservedAt  time.Time `json:"observed_at"`
	Available   bool      `json:"available"`
}
type Status struct {
	State                   string  `json:"state"`
	DeferredReason          string  `json:"deferred_reason,omitempty"`
	MaxWorkers              int     `json:"max_workers"`
	MaxQueue                int     `json:"max_queue"`
	RetentionBytes          int64   `json:"retention_bytes"`
	RetentionScope          string  `json:"retention_scope"`
	OldestJobMaxWaitSeconds float64 `json:"oldest_job_max_wait_seconds"`
	CancelSupported         bool    `json:"cancel_supported"`
	QueueDepth              int     `json:"queue_depth"`
	Running                 int     `json:"running"`
	OldestJobAgeSeconds     float64 `json:"oldest_job_age_seconds"`
	Storage
	Signals
}
type Probe func(context.Context) Signals

var loadPattern = regexp.MustCompile(`\{\s*([0-9]+(?:\.[0-9]+)?)`)
var idlePattern = regexp.MustCompile(`"HIDIdleTime"\s*=\s*([0-9]+)`)

func Native(ctx context.Context) Signals {
	s := Signals{Provider: runtime.GOOS, Power: "unknown", ObservedAt: time.Now().UTC()}
	if runtime.GOOS != "darwin" {
		return s
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	loadObserved := false
	if b, err := exec.CommandContext(bounded, "pmset", "-g", "batt").Output(); err == nil {
		line := string(b)
		if strings.Contains(line, "'AC Power'") {
			s.Power = "ac"
		} else if strings.Contains(line, "'Battery Power'") {
			s.Power = "battery"
		}
	}
	if b, err := exec.CommandContext(bounded, "sysctl", "-n", "vm.loadavg").Output(); err == nil {
		if m := loadPattern.FindSubmatch(b); len(m) > 1 {
			s.Load, _ = strconv.ParseFloat(string(m[1]), 64)
			loadObserved = true
		}
	}
	if b, err := exec.CommandContext(bounded, "ioreg", "-c", "IOHIDSystem", "-d", "4").Output(); err == nil {
		if m := idlePattern.FindSubmatch(b); len(m) > 1 {
			n, _ := strconv.ParseFloat(string(m[1]), 64)
			s.IdleSeconds = n / 1e9
		}
	}
	s.Available = s.Power != "unknown" && loadObserved
	return s
}
func Evaluate(s Signals, maxWait time.Duration) Status {
	if maxWait <= 0 {
		maxWait = MaxWait
	}
	p := Status{State: "normal", MaxWorkers: 1, MaxQueue: 100, RetentionBytes: JournalRetentionBytes, RetentionScope: "maintenance_journal", OldestJobMaxWaitSeconds: maxWait.Seconds(), CancelSupported: true, Signals: s}
	if !s.Available {
		p.DeferredReason = "signals_unavailable"
		return p
	}
	if s.Power == "battery" {
		p.State = "constrained"
		p.DeferredReason = "battery"
	} else if s.Load > float64(runtime.NumCPU())*.75 {
		p.State = "constrained"
		p.DeferredReason = "load"
	} else if s.IdleSeconds >= 120 {
		p.State = "idle_opportunity"
	}
	return p
}
func (s Status) Validate() error {
	if s.MaxWorkers < 1 || s.MaxQueue < 1 || s.RetentionBytes < 1 || s.OldestJobMaxWaitSeconds <= 0 {
		return fmt.Errorf("invalid resource policy")
	}
	return nil
}
