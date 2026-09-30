package webui

import (
	"encoding/base64"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"net/http"
	"strings"
)

func (s *Server) instanceAPI(w http.ResponseWriter, r *http.Request) {
	if (r.Method != http.MethodGet && r.Method != http.MethodPost) || !s.authorised(r, r.Method == http.MethodPost) {
		fail(w, 403, "valid session, origin and CSRF required")
		return
	}
	if r.URL.Path == "/api/v1/instance/logo" && r.Method != http.MethodGet {
		fail(w, 405, "method not allowed")
		return
	}
	var v store.Instance
	var err error
	if r.Method == http.MethodPost {
		var in store.InstanceSettings
		if decode(r, &in) != nil {
			fail(w, 400, "invalid instance request")
			return
		}
		v, err = store.SaveInstance(s.dataDir, in)
	} else {
		v, err = store.LoadInstance(s.dataDir)
	}
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if r.URL.Path == "/api/v1/instance/logo" {
		if v.Logo == "" {
			fail(w, 404, "no logo configured")
			return
		}
		parts := strings.SplitN(v.Logo, ",", 2)
		b, _ := base64.StdEncoding.DecodeString(parts[1])
		w.Header().Set("Content-Type", strings.TrimSuffix(strings.TrimPrefix(parts[0], "data:"), ";base64"))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonBody(w, v)
}
