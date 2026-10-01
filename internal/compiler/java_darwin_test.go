//go:build darwin

package compiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestAppleJavacLauncherReportsUnavailableWithoutExecution(t *testing.T) {
	data := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := Run(ctx, model.Repository{ID: "fixture", Root: t.TempDir()}, []model.File{{Path: "Worker.java", Language: "java", Content: "class Worker {}"}}, model.CompilerRuntime{JavaHome: "/usr"}, data)
	if err != nil || len(result.Diagnostics) != 1 || result.Covered["java"] {
		t.Fatal("launcher falsely provided semantic coverage", err)
	}
	if ctx.Err() != nil {
		t.Fatal("optional frontend opened or stalled in Apple launcher")
	}
	if _, err = os.Stat(filepath.Join(data, "compiler-cache")); !os.IsNotExist(err) {
		t.Fatal("launcher path performed compilation side effects")
	}
}
