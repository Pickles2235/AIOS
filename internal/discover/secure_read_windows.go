//go:build windows

package discover

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

var (
	errNotRegular = errors.New("path is not a regular file")
	errSymlink    = errors.New("path contains a symbolic link or reparse point")
	errTooLarge   = errors.New("file exceeds configured size limit")
)

func readRegularFile(root, rel string, maxBytes int64) ([]byte, error) {
	parts, err := safeRelativeParts(rel)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	for index := range parts {
		candidate := filepath.Join(append([]string{root}, parts[:index+1]...)...)
		attributes, attributeErr := windows.GetFileAttributes(windows.StringToUTF16Ptr(candidate))
		if attributeErr != nil {
			return nil, fmt.Errorf("inspect %q: %w", rel, attributeErr)
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return nil, fmt.Errorf("%w: %q", errSymlink, rel)
		}
	}
	path := filepath.Join(append([]string{root}, parts...)...)
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", rel, err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("open %q: invalid file handle", rel)
	}
	defer file.Close()
	finalPath, err := finalPath(handle)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", rel, err)
	}
	rootPrefix := strings.TrimSuffix(normalizeWindowsPath(root), `\`) + `\`
	if !strings.HasPrefix(strings.ToLower(normalizeWindowsPath(finalPath)), strings.ToLower(rootPrefix)) {
		return nil, fmt.Errorf("%w: %q resolved outside repository", errSymlink, rel)
	}
	var handleInfo windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &handleInfo); err != nil {
		return nil, fmt.Errorf("inspect %q: %w", rel, err)
	}
	if handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return nil, fmt.Errorf("%w: %q", errNotRegular, rel)
	}
	before, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q", errNotRegular, rel)
	}
	if before.Size() > maxBytes {
		return nil, fmt.Errorf("%w: %q", errTooLarge, rel)
	}
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", rel, err)
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(content)) != after.Size() {
		return nil, fmt.Errorf("file %q changed while being read", rel)
	}
	return content, nil
}

func safeRelativeParts(rel string) ([]string, error) {
	if rel == "" || rel == "." || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("unsafe relative path %q", rel)
	}
	parts := strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' })
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return nil, fmt.Errorf("unsafe relative path %q", rel)
		}
	}
	return parts, nil
}

func finalPath(handle windows.Handle) (string, error) {
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	length, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	if length >= uint32(len(buffer)) {
		return "", fmt.Errorf("resolved path exceeds platform limit")
	}
	return windows.UTF16ToString(buffer[:length]), nil
}

func normalizeWindowsPath(path string) string {
	return strings.TrimPrefix(filepath.Clean(path), `\\?\`)
}
