package service

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSniffFileRequiresMatchingExtensionAndContent(t *testing.T) {
	for _, test := range []struct{ name, payload, want string }{
		{"document.PDF", "%PDF-1.7\n", "application/pdf"},
		{"photo.jpg", "\xff\xd8\xff\xe0", "image/jpeg"},
		{"photo.png", "\x89PNG\r\n\x1a\n", "image/png"},
		{"spoofed.pdf", "<html>untrusted</html>", ""},
		{"wrong.jpg", "%PDF-1.7\n", ""},
		{"unknown.bin", "%PDF-1.7\n", ""},
		{"empty.pdf", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(test.payload))
			got, err := sniffFile(test.name, reader)
			if test.want == "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("expected rejection, got %q and %v", got, err)
				}
			} else if err != nil || got != test.want {
				t.Fatalf("expected %q, got %q and %v", test.want, got, err)
			}
		})
	}
}

func TestWriteFileRejectsOneByteOverLimit(t *testing.T) {
	for _, size := range []int{MaxFileSize, MaxFileSize + 1} {
		file, err := os.OpenFile(filepath.Join(t.TempDir(), "file"), os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		count, err := writeFile(file, strings.NewReader(strings.Repeat("x", size)))
		if size == MaxFileSize {
			if err != nil || count != MaxFileSize {
				t.Fatalf("exact file limit should succeed, got %d and %v", count, err)
			}
		} else if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("oversized file should fail, got %v", err)
		}
	}
}
