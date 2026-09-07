package commands

import (
	"encoding/json"
	"os"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewRemoteCommand creates the command group for configuring a repository's
// Rack remote. Remote configuration is portable and non-secret: no
// credential is ever read from or written to repository control state.
func NewRemoteCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	command := &cobra.Command{
		Use:          "remote",
		Short:        "Configure the repository's Rack remote",
		SilenceUsage: true,
	}
	command.AddCommand(
		newRemoteSetCommand(repoProvider),
		newRemoteShowCommand(repoProvider),
		newRemoteRemoveCommand(repoProvider),
		newRemoteBranchCommand(repoProvider),
	)
	return command
}

type remoteConfigResult struct {
	Endpoint    string `json:"endpoint"`
	TenantID    string `json:"tenantId,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	RepoID      string `json:"repoId"`
	AuthMode    string `json:"authMode"`
}

func newRemoteConfigResult(cfg repository.RemoteConfig) remoteConfigResult {
	repoID := cfg.RepoID
	if repoID == "" {
		repoID = cfg.WorkspaceID
	}
	workspaceID := cfg.WorkspaceID
	if workspaceID == "" {
		workspaceID = cfg.RepoID
	}
	return remoteConfigResult{
		Endpoint:    cfg.Endpoint,
		TenantID:    cfg.TenantID,
		WorkspaceID: workspaceID,
		RepoID:      repoID,
		AuthMode:    string(cfg.AuthMode),
	}
}

func newRemoteSetCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var endpoint, tenantID, workspaceID, repoID, authMode string
	command := &cobra.Command{
		Use:          "set",
		Short:        "Configure the repository's Rack remote endpoint, identity, and auth mode",
		Long:         "Persist a non-secret Rack remote configuration. No credential is ever read from or written to repository control state.",
		Example:      "  spl remote set --endpoint https://rack.example.com --tenant-id acme --workspace-id prod --auth-mode bearer\n  spl remote set --endpoint https://rack.example.com --repo-id acme-prod --auth-mode bearer",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			if workspaceID == "" && repoID != "" {
				workspaceID = repoID
			}
			if repoID == "" && workspaceID != "" {
				repoID = workspaceID
			}
			cfg := repository.RemoteConfig{
				Endpoint:    endpoint,
				TenantID:    tenantID,
				WorkspaceID: workspaceID,
				RepoID:      repoID,
				AuthMode:    repository.RemoteAuthMode(authMode),
			}
			if err := repo.SetRemote(cfg); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(newRemoteConfigResult(cfg))
		},
	}
	command.Flags().StringVar(&endpoint, "endpoint", "", "Rack remote HTTP(S) endpoint")
	command.Flags().StringVar(&tenantID, "tenant-id", "", "logical Rack tenant identity")
	command.Flags().StringVar(&tenantID, "tenant", "", "alias for --tenant-id")
	command.Flags().StringVar(&workspaceID, "workspace-id", "", "logical Rack workspace identity")
	command.Flags().StringVar(&workspaceID, "workspace", "", "alias for --workspace-id")
	command.Flags().StringVar(&repoID, "repo-id", "", "legacy Rack repository identity (alias for --workspace-id)")
	command.Flags().StringVar(&authMode, "auth-mode", "", `authentication mode: "bearer" or "api_key"`)
	_ = command.MarkFlagRequired("endpoint")
	_ = command.MarkFlagRequired("auth-mode")
	return command
}

type remoteShowResult struct {
	remoteConfigResult
	VersionStatus string                `json:"versionStatus"`
	Versions      *remote.VersionReport `json:"versions,omitempty"`
	Message       string                `json:"message,omitempty"`
}

func newRemoteShowCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "show",
		Short:        "Show the repository's configured Rack remote and negotiated graphcontract version status",
		Long:         "Print the repository's non-secret Rack remote configuration and probe its /healthz endpoint to compare graphcontract format versions. Never prints credentials.",
		Example:      "  spl remote show",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			cfg, ok, err := repo.Remote()
			if err != nil {
				return err
			}
			if !ok {
				return repository.ErrRemoteNotConfigured
			}
			result := remoteShowResult{remoteConfigResult: newRemoteConfigResult(cfg), VersionStatus: "unreachable"}
			credential := resolveShowCredential(cfg)
			report, err := remote.NegotiateVersions(command.Context(), remote.NewClient(), cfg, credential)
			if err != nil {
				result.Message = remote.Redact(err.Error(), credential, cfg.WorkspaceOrRepoID())
			} else {
				result.VersionStatus = "negotiated"
				result.Versions = &report
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
}

// resolveShowCredential resolves a best-effort credential for the version
// probe from the OS keychain or environment only. It intentionally never
// prompts interactively so `spl remote show` never blocks waiting on input;
// an unresolved credential simply results in an unauthenticated probe.
func resolveShowCredential(cfg repository.RemoteConfig) string {
	credential, err := remote.ResolveCredential(cfg.WorkspaceOrRepoID(), cfg.AuthMode, remote.ResolveOptions{
		Keychain: remote.KeyringStore{},
		Getenv:   os.Getenv,
	})
	if err != nil {
		return ""
	}
	return credential.Value
}

func newRemoteRemoveCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "remove",
		Short:        "Remove the repository's configured Rack remote",
		Example:      "  spl remote remove",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			_, existed, err := repo.Remote()
			if err != nil {
				return err
			}
			if err := repo.RemoveRemote(); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				Removed bool `json:"removed"`
			}{Removed: existed})
		},
	}
}
