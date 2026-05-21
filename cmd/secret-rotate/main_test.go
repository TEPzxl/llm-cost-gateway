package main

import "testing"

func TestParseArgsSupportsDryRunAndOrgID(t *testing.T) {
	opts, err := parseArgs([]string{
		"-config", "configs/prod.yaml",
		"-org-id", "00000000-0000-0000-0000-000000000001",
		"-dry-run",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if opts.configPath != "configs/prod.yaml" {
		t.Fatalf("configPath = %q, want configs/prod.yaml", opts.configPath)
	}
	if opts.orgID == nil || opts.orgID.String() != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("orgID = %v, want parsed uuid", opts.orgID)
	}
	if !opts.dryRun {
		t.Fatal("dryRun = false, want true")
	}
}

func TestParseArgsRejectsInvalidOrgID(t *testing.T) {
	if _, err := parseArgs([]string{"-org-id", "not-a-uuid"}); err == nil {
		t.Fatal("parseArgs returned nil error, want invalid org id error")
	}
}
