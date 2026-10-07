package resourcepolicy

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestPolicyTransitionsAndBounds(t *testing.T) {
	base := Signals{Provider: "test", Power: "ac", Load: 0.1, ObservedAt: time.Now(), Available: true}
	for _, tt := range []struct {
		name       string
		power      string
		load, idle float64
		want       string
	}{
		{"normal", "ac", .1, 1, "normal"}, {"battery", "battery", .1, 1, "constrained"},
		{"load", "ac", float64(runtime.NumCPU()), 1, "constrained"}, {"idle", "ac", .1, 130, "idle_opportunity"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := base
			s.Power = tt.power
			s.Load = tt.load
			s.IdleSeconds = tt.idle
			p := Evaluate(s, MaxWait)
			if p.State != tt.want || p.Validate() != nil || p.OldestJobMaxWaitSeconds != 300 {
				t.Fatal(p)
			}
		})
	}
	unknown := Evaluate(Signals{Provider: "test", Power: "unknown"}, MaxWait)
	if unknown.State != "normal" || unknown.DeferredReason != "signals_unavailable" {
		t.Fatal(unknown)
	}
}
func TestNativeSignalOnDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native Darwin smoke")
	}
	s := Native(context.Background())
	if s.Provider != "darwin" || s.Power == "unknown" || s.Load <= 0 || s.ObservedAt.IsZero() {
		t.Fatal(s)
	}
}
