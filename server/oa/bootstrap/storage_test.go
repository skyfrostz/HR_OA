package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateStorageRejectsPureAdminTree(t *testing.T) {
	repo := t.TempDir()
	server := filepath.Join(repo, "server")
	public := filepath.Join(repo, "web-pure-admin", "public")
	for _, dir := range []string{server, public} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(server, "go.mod"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(server)
	if _, err := prepareStorage(filepath.Join(public, "oa-test-private")); err == nil {
		t.Fatal("private storage must reject paths inside the Pure Admin public tree")
	}

	alias := filepath.Join(repo, "public-alias")
	if err := os.Symlink(public, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareStorage(filepath.Join(alias, "oa-test-private")); err == nil {
		t.Fatal("private storage must also reject symlinks into the Pure Admin tree")
	}
	if _, err := prepareStorage(filepath.Join(repo, "private", "oa")); err != nil {
		t.Fatalf("private storage outside public trees must still work: %v", err)
	}
}
