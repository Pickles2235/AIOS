package store

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxLogoBytes = 256 << 10
const instanceFile = "instance.json"

type InstanceSettings struct {
	Name       string `json:"name"`
	Logo       string `json:"logo,omitempty"`
	SeedColour string `json:"seed_colour"`
}
type Instance struct {
	ID string `json:"id"`
	InstanceSettings
}

var colourPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// NormalizeInstance accepts bounded raster data only, never URLs or executable SVG.
func NormalizeInstance(v InstanceSettings) (InstanceSettings, error) {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || !utf8.ValidString(v.Name) || utf8.RuneCountInString(v.Name) > 80 || strings.IndexFunc(v.Name, unicode.IsControl) >= 0 {
		return v, fmt.Errorf("name must contain 1–80 printable characters")
	}
	if !colourPattern.MatchString(v.SeedColour) {
		return v, fmt.Errorf("seed_colour must be a six-digit hex colour")
	}
	v.SeedColour = strings.ToLower(v.SeedColour)
	if v.Logo == "" {
		return v, nil
	}
	if len(v.Logo) > MaxLogoBytes*2 {
		return v, fmt.Errorf("logo exceeds 256 KiB")
	}
	parts := strings.SplitN(v.Logo, ",", 2)
	if len(parts) != 2 || (parts[0] != "data:image/png;base64" && parts[0] != "data:image/jpeg;base64") {
		return v, fmt.Errorf("logo must be a PNG or JPEG data URL")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(data) > MaxLogoBytes {
		return v, fmt.Errorf("invalid or oversized logo")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 1024 || cfg.Height > 1024 {
		return v, fmt.Errorf("logo dimensions must be within 1024 by 1024")
	}
	if parts[0] != "data:image/"+format+";base64" {
		return v, fmt.Errorf("logo MIME type does not match image")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return v, fmt.Errorf("invalid logo image")
	}
	var b bytes.Buffer
	if format == "png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 90})
	}
	if err != nil || b.Len() > MaxLogoBytes {
		return v, fmt.Errorf("encoded logo exceeds 256 KiB")
	}
	v.Logo = "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(b.Bytes())
	return v, nil
}

func instanceLock(dataDir string) (*writerLock, error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil || resolved != abs {
		return nil, fmt.Errorf("instance directory must be canonical and not contain symlinks")
	}
	if err = validateDataDirectory(abs); err != nil {
		return nil, err
	}
	return acquireWriterLock(filepath.Join(abs, ".instance.lock"))
}
func readInstance(dataDir string) (Instance, error) {
	path := filepath.Join(dataDir, instanceFile)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		b := make([]byte, 16)
		if _, err = rand.Read(b); err != nil {
			return Instance{}, err
		}
		return Instance{ID: hex.EncodeToString(b), InstanceSettings: InstanceSettings{Name: "AgentOS", SeedColour: "#5865f2"}}, nil
	}
	if err != nil {
		return Instance{}, err
	}
	if err = validateOwnedRegularFile(path, info); err != nil {
		return Instance{}, err
	}
	if info.Size() > MaxLogoBytes*2 || info.Mode().Perm() != 0600 {
		return Instance{}, fmt.Errorf("instance metadata must be bounded and owner-only")
	}
	f, err := os.Open(path)
	if err != nil {
		return Instance{}, err
	}
	defer f.Close()
	var v Instance
	d := json.NewDecoder(io.LimitReader(f, MaxLogoBytes*2))
	d.DisallowUnknownFields()
	if err = d.Decode(&v); err != nil {
		return v, err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return v, fmt.Errorf("invalid instance metadata")
	}
	if len(v.ID) != 32 {
		return v, fmt.Errorf("invalid instance identity")
	}
	if _, err = hex.DecodeString(v.ID); err != nil {
		return v, err
	}
	_, err = NormalizeInstance(v.InstanceSettings)
	return v, err
}
func writeInstance(dataDir string, v Instance) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dataDir, ".instance-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(append(b, '\n')); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, filepath.Join(dataDir, instanceFile)); err != nil {
		return err
	}
	dir, err := os.Open(dataDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// LoadInstance lazily migrates legacy data directories without changing Knowledge IR.
func ReadExistingInstance(dataDir string) (Instance, error) {
	if _, err := os.Lstat(filepath.Join(dataDir, instanceFile)); err != nil {
		return Instance{}, err
	}
	return readInstance(dataDir)
}
func LoadInstance(dataDir string) (Instance, error) {
	lock, err := instanceLock(dataDir)
	if err != nil {
		return Instance{}, err
	}
	defer lock.close()
	v, err := readInstance(dataDir)
	if err != nil {
		return v, err
	}
	if _, err = os.Lstat(filepath.Join(dataDir, instanceFile)); os.IsNotExist(err) {
		err = writeInstance(dataDir, v)
	}
	return v, err
}
func SaveInstance(dataDir string, settings InstanceSettings) (Instance, error) {
	settings, err := NormalizeInstance(settings)
	if err != nil {
		return Instance{}, err
	}
	lock, err := instanceLock(dataDir)
	if err != nil {
		return Instance{}, err
	}
	defer lock.close()
	v, err := readInstance(dataDir)
	if err != nil {
		return v, err
	}
	v.InstanceSettings = settings
	return v, writeInstance(dataDir, v)
}
