package main

import (
	"errors"
	"flag"
	"io"
)

type commandOptions struct {
	checkConfig        bool
	version            bool
	migrateAttachments bool
	apply              bool
	afterID            uint
	limit              int
}

func parseOptions(args []string, output io.Writer) (commandOptions, error) {
	var opts commandOptions
	flags := flag.NewFlagSet("hr-oa", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.BoolVar(&opts.checkConfig, "check-config", false, "Validate configuration without database or provider access")
	flags.BoolVar(&opts.version, "version", false, "Print build version")
	flags.BoolVar(&opts.migrateAttachments, "migrate-attachments", false, "Check a batch of local attachments for migration to configured OSS")
	flags.BoolVar(&opts.apply, "apply", false, "Upload, verify and switch attachments; requires --migrate-attachments")
	flags.UintVar(&opts.afterID, "after-id", 0, "Start after this attachment ID")
	flags.IntVar(&opts.limit, "limit", 100, "Attachment batch size (1 to 500)")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, errors.New("unexpected positional arguments")
	}
	modes := 0
	for _, enabled := range []bool{opts.checkConfig, opts.version, opts.migrateAttachments} {
		if enabled {
			modes++
		}
	}
	if modes > 1 {
		return opts, errors.New("choose only one command mode")
	}
	migrationOptions := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "apply" || f.Name == "after-id" || f.Name == "limit" {
			migrationOptions = true
		}
	})
	if migrationOptions && !opts.migrateAttachments {
		return opts, errors.New("migration options require --migrate-attachments")
	}
	if opts.limit < 1 || opts.limit > 500 {
		return opts, errors.New("limit must be between 1 and 500")
	}
	return opts, nil
}
