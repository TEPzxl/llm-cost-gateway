package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	rotation "github.com/tep/llm-cost-gateway/internal/admin"
	"github.com/tep/llm-cost-gateway/internal/config"
	"github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/store"
)

type options struct {
	configPath string
	orgID      *uuid.UUID
	dryRun     bool
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	keys, err := cfg.SecretEncryptionKeys()
	if err != nil {
		return err
	}
	version := cfg.SecretEncryptionKeyVersion
	if version <= 0 {
		version = 1
	}
	keyRing, err := crypto.NewSecretKeyRing(int32(version), keys)
	if err != nil {
		return err
	}
	service := rotation.NewSecretRotationService(st, keyRing)
	providerResult, err := service.ReencryptProviderSecrets(ctx, rotation.ReencryptProviderSecretsParams{
		OrgID:  opts.orgID,
		DryRun: opts.dryRun,
	})
	if err != nil {
		return err
	}
	webhookResult, err := service.ReencryptWebhookSecrets(ctx, rotation.ReencryptWebhookSecretsParams{
		OrgID:  opts.orgID,
		DryRun: opts.dryRun,
	})
	if err != nil {
		return err
	}
	fmt.Printf("provider secrets scanned=%d rotated=%d skipped=%d dry_run=%t\n", providerResult.Scanned, providerResult.Rotated, providerResult.Skipped, providerResult.DryRun)
	fmt.Printf("webhook secrets scanned=%d rotated=%d skipped=%d dry_run=%t\n", webhookResult.Scanned, webhookResult.Rotated, webhookResult.Skipped, webhookResult.DryRun)
	return nil
}

func parseArgs(args []string) (options, error) {
	fs := flag.NewFlagSet("secret-rotate", flag.ContinueOnError)
	var opts options
	var orgID string
	fs.StringVar(&opts.configPath, "config", "", "config file path")
	fs.StringVar(&orgID, "org-id", "", "optional organization id")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "show planned rotation without writing changes")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if orgID != "" {
		parsed, err := uuid.Parse(orgID)
		if err != nil {
			return options{}, fmt.Errorf("parse org-id: %w", err)
		}
		opts.orgID = &parsed
	}
	return opts, nil
}
