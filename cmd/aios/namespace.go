package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/catalog"
	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
	"github.com/AdamNi-7080/AIOS/internal/localname"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"github.com/AdamNi-7080/AIOS/internal/webui"
)

// Verification owns a disposable server/name, never the default installation.
func runNamespace(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "verify" {
		return fmt.Errorf("namespace requires verify")
	}
	fs := flag.NewFlagSet("namespace verify", flag.ContinueOnError)
	name := fs.String("name", "", "selected local namespace")
	_ = fs.Bool("json", false, "structured output")
	if e := fs.Parse(args[1:]); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected namespace arguments")
	}
	if e := localname.Validate(*name); e != nil {
		return e
	}
	if e := lifecycle.RequireNative(); e != nil {
		return e
	}
	temp, e := os.MkdirTemp("", "aios-namespace-verify-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	temp, e = filepath.EvalSymlinks(temp)
	if e != nil {
		return e
	}
	w, e := store.OpenWriter(temp)
	if e != nil {
		return e
	}
	if e = w.Close(); e != nil {
		return e
	}
	db, e := store.OpenReadOnly(temp)
	if e != nil {
		return e
	}
	defer db.Close()
	s, e := webui.New(catalog.Config{}, db, nil)
	if e != nil {
		return e
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	go func() { _ = s.Serve(ctx) }()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Transport: &http.Transport{Proxy: nil}, Timeout: 8 * time.Second}
	defer client.CloseIdleConnections()
	post := func(origin, path string, body any, csrf string) (map[string]any, error) {
		b, _ := json.Marshal(body)
		r, _ := http.NewRequestWithContext(ctx, "POST", origin+path, bytes.NewReader(b))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		resp, e := client.Do(r)
		if e != nil {
			return nil, fmt.Errorf("native namespaced HTTP request unavailable")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("native namespaced HTTP boundary rejected (%d)", resp.StatusCode)
		}
		var out map[string]any
		e = json.NewDecoder(resp.Body).Decode(&out)
		return out, e
	}
	auth := func(link string) (string, string, error) {
		u, e := url.Parse(link)
		if e != nil {
			return "", "", e
		}
		token, e := url.ParseQuery(u.Fragment)
		if e != nil {
			return "", "", e
		}
		origin := u.Scheme + "://" + u.Host
		out, e := post(origin, "/api/v1/session", map[string]string{"token": token.Get("token")}, "")
		if e != nil {
			return "", "", e
		}
		csrf, ok := out["csrf_token"].(string)
		if !ok {
			return "", "", fmt.Errorf("missing native session")
		}
		return origin, csrf, nil
	}
	origin, csrf, e := auth(s.URL())
	if e != nil {
		return e
	}
	state, e := post(origin, "/api/v1/namespace", map[string]string{"namespace": *name}, csrf)
	if e != nil {
		return e
	}
	if state["native_dns_sd"] != true {
		return fmt.Errorf("native namespace registration not active")
	}
	link := s.FreshURL()
	u, e := url.Parse(link)
	if e != nil {
		return e
	}
	addresses, e := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
	if e != nil || len(addresses) == 0 {
		return fmt.Errorf("native namespace did not resolve")
	}
	for _, a := range addresses {
		if !a.IP.IsLoopback() {
			return fmt.Errorf("native namespace resolves off loopback")
		}
	}
	named, _, e := auth(link)
	if e != nil {
		return e
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", named+"/api/v1/status", nil)
	resp, e := client.Do(req)
	if e != nil {
		return e
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("native secure origin unavailable")
	}
	req, _ = http.NewRequestWithContext(ctx, "GET", named+"/api/v1/status", nil)
	req.Host = "foreign.example:" + u.Port()
	resp, e = client.Do(req)
	if e != nil {
		return e
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		return fmt.Errorf("foreign Host accepted")
	}
	req, _ = http.NewRequestWithContext(ctx, "GET", named+"/api/v1/status", nil)
	req.Header.Set("Origin", origin)
	resp, e = client.Do(req)
	if e != nil {
		return e
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		return fmt.Errorf("cross-origin alias accepted")
	}
	recovery, _ := url.Parse(s.FreshURL())
	recovery.Host = net.JoinHostPort("localhost", recovery.Port())
	recovered, _, e := auth(recovery.String())
	if e != nil {
		return e
	}
	req, _ = http.NewRequestWithContext(ctx, "GET", recovered+"/api/v1/status", nil)
	resp, e = client.Do(req)
	if e != nil {
		return e
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("localhost recovery unavailable")
	}
	return writeJSON(map[string]any{"namespace": *name, "native_dns_sd": true, "resolved_loopback": true, "http_origin_verified": true, "localhost_recovery_verified": true, "foreign_host_rejected": true, "foreign_origin_rejected": true, "purpose": "disposable_native_namespace_verification"})
}
