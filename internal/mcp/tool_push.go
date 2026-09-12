package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolPush(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_push",
		Description: "Push verified local commits for a branch to the repository's configured Rack remote. Returns rejection details if the remote branch has advanced (pull and merge locally before retrying).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to push",
				},
				"base_commit": map[string]any{
					"type":        "string",
					"description": "Optional base commit known to remote",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch     string `json:"branch"`
				BaseCommit string `json:"base_commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}

			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				cfg, ok, err := repo.Remote()
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, repository.ErrRemoteNotConfigured
				}

				credential, err := remote.ResolveCredential(cfg.WorkspaceOrRepoID(), cfg.AuthMode, remote.ResolveOptions{
					Keychain: remote.KeyringStore{},
					Getenv:   os.Getenv,
				})
				if err != nil {
					return nil, fmt.Errorf("resolve credential: %w", err)
				}

				client := remote.NewClient()
				pack, err := repo.BuildPushPack(ctx, in.Branch, in.BaseCommit)
				if err != nil {
					if errors.Is(err, repository.ErrNothingToPush) {
						return map[string]any{
							"branch":  in.Branch,
							"pushed":  false,
							"message": "nothing to push",
						}, nil
					}
					return nil, err
				}

				if len(pack.AssetHashes) > 0 {
					negRes, negErr := remote.NegotiateAssets(ctx, client, cfg, credential.Value, remote.AssetNegotiationRequest{
						Hashes: pack.AssetHashes,
					})
					if negErr != nil {
						return nil, fmt.Errorf("negotiate assets: %w", negErr)
					}
					for _, missingHash := range negRes.Missing {
						reader, _, _, readErr := repo.ReadAsset(ctx, in.Branch, missingHash)
						if readErr != nil {
							return nil, fmt.Errorf("open missing asset %s: %w", missingHash, readErr)
						}
						uploadErr := remote.UploadAsset(ctx, client, cfg, credential.Value, missingHash, "application/octet-stream", reader)
						_ = reader.Close()
						if uploadErr != nil {
							return nil, fmt.Errorf("upload asset %s: %w", missingHash, uploadErr)
						}
					}
				}

				remoteCommits := make([]remote.PushCommitRecord, len(pack.Commits))
				for i, commit := range pack.Commits {
					remoteCommits[i] = remote.PushCommitRecord{ID: commit.ID, Commit: commit.Commit}
				}

				req := remote.PushRequest{
					Branch:       pack.Branch,
					BaseCommit:   pack.BaseCommit,
					TargetCommit: pack.TargetCommit,
					Commits:      remoteCommits,
					PackHash:     pack.PackHash,
					PackFormat:   uint32(pack.PackFormat),
					PackData:     pack.PackData,
					AssetHashes:  pack.AssetHashes,
				}
				result, err := remote.Push(ctx, client, cfg, credential.Value, req)
				if err != nil {
					var nffErr *remote.NonFastForwardError
					if errors.As(err, &nffErr) {
						return map[string]any{
							"branch":        in.Branch,
							"pushed":        false,
							"rejected":      true,
							"actualHead":    nffErr.ActualHead,
							"message":       nffErr.Guidance,
							"correlationId": nffErr.CorrelationID,
						}, nil
					}
					return nil, err
				}
				if err := repo.SetRemoteBranchTracking(in.Branch, result.Branch, result.HeadCommit); err != nil {
					return nil, fmt.Errorf("update remote branch tracking: %w", err)
				}
				return result, nil
			})
		},
	}
}
