package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

type cloneResult struct {
	Cloned           bool   `json:"cloned"`
	Directory        string `json:"directory"`
	Endpoint         string `json:"endpoint"`
	TenantID         string `json:"tenantId,omitempty"`
	WorkspaceID      string `json:"workspaceId,omitempty"`
	RepoID           string `json:"repoId,omitempty"`
	Branch           string `json:"branch"`
	HeadCommit       string `json:"headCommit,omitempty"`
	CommitsInstalled int    `json:"commitsInstalled"`
	Empty            bool   `json:"empty,omitempty"`
}

// NewCloneCommand creates the `spl clone` command to clone a remote workspace
// from Spool Rack into a new local directory.
func NewCloneCommand() *cobra.Command {
	var (
		endpoint    string
		tenantID    string
		workspaceID string
		repoID      string
		branch      string
		authMode    string
	)

	command := &cobra.Command{
		Use:   "clone [url] [directory]",
		Short: "Clone a remote workspace from Spool Rack into a local directory",
		Long: "Clone initializes a new Spool workspace locally, configures its Rack remote, " +
			"downloads the full graph history for the specified branch (or remote default branch), " +
			"and materializes its state so you can immediately begin pulling and pushing ideas.\n\n" +
			"The remote can be specified as a URL (e.g. http://127.0.0.1:8080/api/v1/workspaces/<id>) " +
			"or with --endpoint and --workspace-id flags. If directory is omitted, it defaults to the workspace ID or name.",
		Example: "  spl clone http://127.0.0.1:8080/api/v1/workspaces/ws-backend\n" +
			"  spl clone http://127.0.0.1:8080/workspaces/ws-backend my-backend\n" +
			"  spl clone --endpoint http://127.0.0.1:8080 --tenant-id acme --workspace-id core-graph",
		Args:         cobra.RangeArgs(0, 2),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			ctx := command.Context()

			var targetDir string
			parsedURL := false

			if len(args) > 0 {
				arg := args[0]
				if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
					parsedURL = true
					u, err := url.Parse(arg)
					if err != nil {
						return fmt.Errorf("invalid clone URL %q: %w", arg, err)
					}
					if endpoint == "" {
						endpoint = u.Scheme + "://" + u.Host
					}
					parseURLPathAndQuery(u, &tenantID, &workspaceID, &repoID, &branch, &authMode)
					if len(args) > 1 {
						targetDir = args[1]
					}
				} else {
					targetDir = arg
				}
			}

			if authMode == "" {
				authMode = "bearer"
			}

			cfg := repository.RemoteConfig{
				Endpoint:    endpoint,
				TenantID:    tenantID,
				WorkspaceID: workspaceID,
				RepoID:      repoID,
				AuthMode:    repository.RemoteAuthMode(authMode),
			}

			if err := cfg.Validate(); err != nil {
				if parsedURL && cfg.WorkspaceOrRepoID() == "" {
					return fmt.Errorf("%w: could not extract workspace ID from URL %q", repository.ErrRemoteInvalidConfig, args[0])
				}
				return err
			}

			if targetDir == "" {
				targetDir = cfg.WorkspaceOrRepoID()
			}
			if targetDir == "" {
				return errors.New("destination directory could not be determined; specify [directory] explicitly")
			}

			absDestDir, err := filepath.Abs(targetDir)
			if err != nil {
				return fmt.Errorf("resolve destination directory: %w", err)
			}

			if info, statErr := os.Stat(absDestDir); statErr == nil {
				if !info.IsDir() {
					return fmt.Errorf("destination path %q already exists and is not a directory", absDestDir)
				}
				entries, readErr := os.ReadDir(absDestDir)
				if readErr != nil {
					return fmt.Errorf("inspect destination directory %q: %w", absDestDir, readErr)
				}
				if len(entries) > 0 {
					return fmt.Errorf("destination path %q already exists and is not an empty directory", absDestDir)
				}
			} else if !os.IsNotExist(statErr) {
				return fmt.Errorf("inspect destination path %q: %w", absDestDir, statErr)
			}

			stateDir := filepath.Join(absDestDir, ".spl")

			credential, err := resolvePushCredential(cfg)
			if err != nil {
				// Don't fail immediately on missing credential if remote doesn't require it (e.g. dev mode)
				credential = ""
			}

			client := remote.NewClient()
			cloneRes, err := remote.Clone(ctx, client, cfg, credential, branch)
			if err != nil {
				return writeRemoteErrorEnvelope(command, "clone from rack", err, credential)
			}

			repo, commitsInstalled, err := repository.InitializeClonedRepository(stateDir, cfg, cloneRes.Branch, cloneRes.HeadCommit, cloneRes.Packs)
			if err != nil {
				return fmt.Errorf("initialize cloned workspace: %w", err)
			}
			if closeErr := repo.Close(); closeErr != nil {
				return fmt.Errorf("finalize cloned workspace: %w", closeErr)
			}

			return json.NewEncoder(command.OutOrStdout()).Encode(cloneResult{
				Cloned:           true,
				Directory:        absDestDir,
				Endpoint:         cfg.Endpoint,
				TenantID:         cfg.TenantID,
				WorkspaceID:      cfg.WorkspaceID,
				RepoID:           cfg.RepoID,
				Branch:           cloneRes.Branch,
				HeadCommit:       cloneRes.HeadCommit,
				CommitsInstalled: commitsInstalled,
				Empty:            cloneRes.Empty,
			})
		},
	}

	command.Flags().StringVar(&endpoint, "endpoint", "", "Rack remote HTTP(S) endpoint")
	command.Flags().StringVar(&tenantID, "tenant-id", "", "logical Rack tenant identity")
	command.Flags().StringVar(&tenantID, "tenant", "", "alias for --tenant-id")
	command.Flags().StringVar(&workspaceID, "workspace-id", "", "logical Rack workspace identity")
	command.Flags().StringVar(&workspaceID, "workspace", "", "alias for --workspace-id")
	command.Flags().StringVar(&repoID, "repo-id", "", "deprecated legacy Rack repository identity")
	command.Flags().StringVarP(&branch, "branch", "b", "", "remote branch to clone (defaults to remote default branch)")
	command.Flags().StringVar(&authMode, "auth-mode", "bearer", `authentication mode: "bearer" or "api_key"`)

	return command
}

var (
	reTenantsWorkspaces = regexp.MustCompile(`(?:/api/v1)?/tenants/([^/]+)/workspaces/([^/]+)`)
	reWorkspaces        = regexp.MustCompile(`(?:/api/v1)?/workspaces/([^/]+)`)
	reRepos             = regexp.MustCompile(`(?:/api/v1)?/repos/([^/]+)`)
)

func parseURLPathAndQuery(u *url.URL, tenantID, workspaceID, repoID, branch, authMode *string) {
	path := strings.TrimRight(u.Path, "/")

	if matches := reTenantsWorkspaces.FindStringSubmatch(path); len(matches) == 3 {
		if *tenantID == "" {
			*tenantID = matches[1]
		}
		if *workspaceID == "" {
			*workspaceID = matches[2]
		}
	} else if matches := reWorkspaces.FindStringSubmatch(path); len(matches) == 2 {
		if *workspaceID == "" {
			*workspaceID = matches[1]
		}
	} else if matches := reRepos.FindStringSubmatch(path); len(matches) == 2 {
		if *repoID == "" {
			*repoID = matches[1]
		}
	}

	q := u.Query()
	if *tenantID == "" {
		if t := q.Get("tenant"); t != "" {
			*tenantID = t
		} else if t := q.Get("tenant_id"); t != "" {
			*tenantID = t
		}
	}
	if *branch == "" {
		if b := q.Get("branch"); b != "" {
			*branch = b
		}
	}
	if *authMode == "" {
		if a := q.Get("auth_mode"); a != "" {
			*authMode = a
		}
	}
}
