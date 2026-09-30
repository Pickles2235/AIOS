//go:build windows

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const writerLockName = ".writer.lock"

var ErrWriterLocked = errors.New("index already has an active writer")

type writerLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireWriterLock(path string) (*writerLock, error) {
	if err := rejectReparsePoint(path, true); err != nil {
		return nil, err
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("open writer lock: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("open writer lock: invalid file handle")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("writer lock must be a regular file: %s", path)
	}
	if err = securePath(path); err != nil {
		file.Close()
		return nil, fmt.Errorf("secure writer lock: %w", err)
	}
	lock := &writerLock{file: file}
	if err = windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &lock.overlapped); err != nil {
		file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, ErrWriterLocked
		}
		return nil, fmt.Errorf("acquire writer lock: %w", err)
	}
	return lock, nil
}

func (lock *writerLock) close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	handle := windows.Handle(lock.file.Fd())
	unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &lock.overlapped)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release writer lock: %w", unlockErr)
	}
	return closeErr
}

func validateDataDirectory(path string) error {
	if err := rejectReparsePoint(path, false); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("data directory must be a real directory: %s", path)
	}
	return securePath(path)
}

func validateReadOnlyDatabase(dataDir string) (string, error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	if err = validatePrivatePath(abs, true); err != nil {
		return "", fmt.Errorf("unsafe data directory: %w", err)
	}
	path := filepath.Join(abs, DatabaseName)
	if err = validatePrivatePath(path, false); err != nil {
		return "", fmt.Errorf("unsafe index: %w", err)
	}
	return path, nil
}

func prepareDatabaseFile(path string) error {
	if err := rejectReparsePoint(path, true); err != nil {
		return err
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return fmt.Errorf("open database safely: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		windows.CloseHandle(handle)
		return fmt.Errorf("open database safely: invalid file handle")
	}
	if err = file.Close(); err != nil {
		return err
	}
	return securePath(path)
}

func enforceDatabasePermissions(path string) error {
	for _, candidate := range []string{path, path + "-journal", path + "-shm", path + "-wal"} {
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := validateOwnedRegularFile(candidate, nil); err != nil {
			return err
		}
		if err := securePath(candidate); err != nil {
			return err
		}
	}
	return nil
}

func validateOwnedRegularFile(path string, info os.FileInfo) error {
	if err := rejectReparsePoint(path, false); err != nil {
		return err
	}
	if info == nil {
		var err error
		info, err = os.Stat(path)
		if err != nil {
			return err
		}
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("file must be regular: %s", path)
	}
	return validateOwner(path)
}

func rejectReparsePoint(path string, allowMissing bool) error {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(pointer)
	if allowMissing && errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect path: %w", err)
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("path must not be a reparse point: %s", path)
	}
	return nil
}

func currentUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}

func securePath(path string) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;;FA;;;%s)(A;;FA;;;SY)", sid.String()))
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func validateOwner(path string) error {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	current, err := currentUserSID()
	if err != nil {
		return err
	}
	if owner == nil || !owner.Equals(current) {
		return fmt.Errorf("path must be owned by the current user: %s", path)
	}
	return nil
}

func validatePrivatePath(path string, directory bool) error {
	if err := rejectReparsePoint(path, false); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if directory != info.IsDir() || !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("unexpected path type: %s", path)
	}
	if err = validateOwner(path); err != nil {
		return err
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 2 {
		return fmt.Errorf("path must have the private V1 ACL: %s", path)
	}
	current, err := currentUserSID()
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	seenCurrent, seenSystem := false, false
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(dacl, index, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask&windows.GENERIC_ALL == 0 {
			return fmt.Errorf("path must have only full-control V1 ACL entries: %s", path)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		seenCurrent = seenCurrent || sid.Equals(current)
		seenSystem = seenSystem || sid.Equals(system)
		if !sid.Equals(current) && !sid.Equals(system) {
			return fmt.Errorf("path ACL grants access outside the current user and local system: %s", path)
		}
	}
	if !seenCurrent || !seenSystem {
		return fmt.Errorf("path ACL is missing required V1 principals: %s", path)
	}
	return nil
}
