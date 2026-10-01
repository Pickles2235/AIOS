package webui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"github.com/AdamNi-7080/AIOS/internal/store"
	"image"
	"image/color"
	"image/png"
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
	if r.URL.Path == "/api/v1/instance/logo/generate" {
		if r.Method != http.MethodPost {
			fail(w, 405, "method not allowed")
			return
		}
		var in struct {
			Seed string `json:"seed"`
		}
		if decode(r, &in) != nil || len(in.Seed) > 80 || strings.ContainsAny(in.Seed, "\x00\n\r") {
			fail(w, 400, "logo seed must be bounded printable text")
			return
		}
		v, err = store.LoadInstance(s.dataDir)
		if err != nil {
			fail(w, 500, "instance unavailable")
			return
		}
		seed := sha256.Sum256([]byte(v.ID + ":" + in.Seed))
		var red, green, blue uint8
		_, _ = fmt.Sscanf(v.SeedColour, "#%02x%02x%02x", &red, &green, &blue)
		img := image.NewNRGBA(image.Rect(0, 0, 80, 80))
		bg := color.NRGBA{R: 245, G: 247, B: 250, A: 255}
		fg := color.NRGBA{R: red, G: green, B: blue, A: 255}
		for y := 0; y < 80; y++ {
			for x := 0; x < 80; x++ {
				img.Set(x, y, bg)
			}
		}
		for y := 0; y < 5; y++ {
			for x := 0; x < 3; x++ {
				if seed[y*3+x]&1 != 0 {
					for _, col := range []int{x, 4 - x} {
						for py := 8 + y*12; py < 20+y*12; py++ {
							for px := 10 + col*12; px < 22+col*12; px++ {
								img.Set(px, py, fg)
							}
						}
					}
				}
			}
		}
		var b bytes.Buffer
		if png.Encode(&b, img) != nil {
			fail(w, 500, "logo generation failed")
			return
		}
		v.Logo = "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
		v, err = store.SaveInstance(s.dataDir, v.InstanceSettings)
		if err != nil {
			fail(w, 500, "unable to save generated logo")
			return
		}
		jsonBody(w, map[string]any{"mime": "image/png", "instance": v, "generated_locally": true})
		return
	}
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
