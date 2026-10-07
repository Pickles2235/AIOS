package resourcepolicy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// The admission budget caps owned build starts. Existing canonical/history
// data is never purged to satisfy it. The reserve covers one bounded capture
// and its projections; a failed preflight leaves last-good data untouched.
const MaxOwnedBytes int64 = 100 << 30
const CaptureReserveBytes int64 = 8 << 30
const MinimumFreeBytes int64 = 8 << 30

var ErrStorageBudget = errors.New("owned storage admission budget reached")
var ErrStoragePressure = errors.New("owned volume has insufficient free space")

type Storage struct {
	OwnedBytes        int64     `json:"owned_bytes"`
	AvailableBytes    int64     `json:"available_bytes"`
	MaxOwnedBytes     int64     `json:"max_owned_bytes"`
	StorageState      string    `json:"storage_state"`
	StorageObservedAt time.Time `json:"storage_observed_at"`
}

func AvailableBytes(root string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
func CheckBudget(root string, maxBytes, freeReserve, captureReserve int64) (Storage, error) {
	out := Storage{MaxOwnedBytes: maxBytes, StorageState: "available", StorageObservedAt: time.Now().UTC()}
	if maxBytes <= captureReserve || captureReserve <= 0 || freeReserve < 0 {
		return Storage{StorageState: "unavailable"}, fs.ErrInvalid
	}
	var err error
	out.AvailableBytes, err = AvailableBytes(root)
	if err != nil {
		out.StorageState = "unavailable"
		return out, err
	}
	count := 0
	err = filepath.WalkDir(root, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > 25_000_000 {
			return ErrStorageBudget
		}
		if d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if info.Mode().IsRegular() {
			out.OwnedBytes += info.Size()
		}
		return nil
	})
	if err != nil {
		out.StorageState = "unavailable"
		return out, err
	}
	if out.OwnedBytes > maxBytes-captureReserve {
		out.StorageState = "budget"
		return out, ErrStorageBudget
	}
	if out.AvailableBytes < freeReserve+captureReserve {
		out.StorageState = "pressure"
		return out, ErrStoragePressure
	}
	return out, nil
}
