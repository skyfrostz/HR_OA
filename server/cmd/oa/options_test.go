package main

import (
	"io"
	"testing"
)

func TestMaintenanceModesCannotAccidentallyWrite(t *testing.T) {
	for _, args := range [][]string{{"--apply"}, {"--version", "--migrate-attachments"}, {"--check-config", "--apply"}, {"--after-id=0"}, {"--migrate-attachments", "--limit=0"}, {"--migrate-attachments", "--limit=501"}} {
		if _, err := parseOptions(args, io.Discard); err == nil {
			t.Fatalf("unsafe options accepted: %v", args)
		}
	}
	for _, args := range [][]string{nil, {"--migrate-attachments"}, {"--check-config"}} {
		opts, err := parseOptions(args, io.Discard)
		if err != nil || opts.apply {
			t.Fatalf("default must not apply: %v", args)
		}
	}
	opts, err := parseOptions([]string{"--migrate-attachments", "--after-id=7", "--limit=20", "--apply"}, io.Discard)
	if err != nil || !opts.apply || opts.afterID != 7 || opts.limit != 20 {
		t.Fatal("explicit migration intent lost")
	}
}
