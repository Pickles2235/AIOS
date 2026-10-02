package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/knowledge"
	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// NewUpgradeHealth creates a temporary read-only candidate server. It does not
// restore setup/removal/namespace, start maintenance, or repair any metadata.
// The caller must have verified the live owned upgrade transaction first.
func NewUpgradeHealth(db *store.Store) (*Server, error) {
	if db == nil {
		return nil, fmt.Errorf("upgrade health requires read-only canonical store")
	}
	var listener net.Listener
	var e error
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		listener, e = listenDualLoopback()
	} else {
		listener, e = net.Listen("tcp4", "127.0.0.1:0")
	}
	if e != nil {
		return nil, e
	}
	data := filepath.Dir(db.Path())
	if _, e = store.ReadExistingInstance(data); e != nil {
		listener.Close()
		return nil, e
	}
	cfg := catalog.Config{Version: 1, Limits: catalog.Defaults()}
	setup := Setup{State: "ready", Active: cfg}
	f, e := lifecycle.OpenBoundedMetadata(filepath.Join(data, "setup.json"), 1<<20)
	if e == nil {
		d := json.NewDecoder(io.LimitReader(f, 1<<20))
		d.DisallowUnknownFields()
		e = d.Decode(&setup)
		if e == nil && d.Decode(&struct{}{}) != io.EOF {
			e = fmt.Errorf("invalid saved setup")
		}
		f.Close()
		if e != nil {
			listener.Close()
			return nil, e
		}
		cfg = setup.Active
	} else if !os.IsNotExist(e) {
		listener.Close()
		return nil, e
	}
	if len(cfg.Sources) == 0 {
		snapshots, e := db.Status(context.Background(), "")
		if e != nil {
			listener.Close()
			return nil, e
		}
		for _, snap := range snapshots {
			cfg.Sources = append(cfg.Sources, catalog.Source{Kind: "repository", ID: snap.RepoID})
		}
	}
	return &Server{db: db, dataDir: data, listener: listener, origin: "http://" + listener.Addr().String(), capability: token(), read: knowledge.New(cfg, db), setup: setup, upgradeHealth: true}, nil
}
func (s *Server) upgradeHealthAPI(w http.ResponseWriter, r *http.Request) bool {
	if !s.upgradeHealth || r.URL.Path == "/api/v1/session" {
		return false
	}
	if !s.authorised(r, r.Method != http.MethodGet) {
		fail(w, 403, "valid local session required")
		return true
	}
	switch r.URL.Path {
	case "/api/v1/query", "/api/v1/entity", "/api/v1/evidence", "/api/v1/neighbors", "/api/v1/projection", "/api/v1/status", "/api/v1/runtime", "/api/v1/daemon/status":
		return false
	case "/api/v1/instance":
		if r.Method == http.MethodGet {
			instance, e := store.ReadExistingInstance(s.dataDir)
			if e != nil {
				fail(w, 503, "instance metadata unavailable")
			} else {
				jsonBody(w, instance)
			}
			return true
		}
	}
	fail(w, 503, "Update validation is read-only. Reconnect after the update completes.")
	return true
}
