package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func prepareStorage(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("OA private storage directory is required")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", errors.New("invalid OA storage directory")
	}
	// Resolve existing ancestors before creating anything so a symlink into a static
	// directory cannot redirect the creation of an otherwise private-looking path.
	dir, err = resolveStoragePath(dir)
	if err != nil || overlapsWebRoots(dir) {
		return "", errors.New("OA storage must be outside static directories")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errors.New("OA private storage directory could not be created")
	}
	entry, err := os.Lstat(dir)
	if err != nil || !entry.IsDir() || entry.Mode().Perm() != 0o700 {
		return "", errors.New("OA storage directory requires mode 0700")
	}
	return dir, nil
}

func resolveStoragePath(dir string) (string, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if !errors.Is(err, os.ErrNotExist) {
		return resolved, err
	}
	parent, err := resolveStoragePath(filepath.Dir(dir))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(dir)), nil
}

func overlapsWebRoots(dir string) bool {
	wd, err := os.Getwd()
	if err != nil {
		return true
	}
	// The controlled runtime mounts no file roots. Guard the upstream web build
	// and upload directories when launched from within the source checkout.
	for root := wd; ; root = filepath.Dir(root) {
		if _, err := os.Stat(filepath.Join(root, "server", "go.mod")); err == nil {
			for _, relative := range []string{"web", "web-pure-admin", "server/uploads", "server/resource"} {
				public := filepath.Join(root, relative)
				resolved, err := resolveStoragePath(public)
				if err != nil {
					return true
				}
				if pathContains(resolved, dir) || pathContains(dir, resolved) {
					return true
				}
			}
			return false
		}
		if root == filepath.Dir(root) {
			return false
		}
	}
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
