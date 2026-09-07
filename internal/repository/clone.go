package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// InitializeClonedRepository creates and initializes a new repository in stateDir
// from a remote clone payload, installing all commits and graph snapshots carried
// by packs, setting up remote configuration, and making branch the active branch.
// If packs is empty (the remote workspace has no commits yet), the repository is
// seeded with the standard initial skeleton.
func InitializeClonedRepository(stateDir string, cfg RemoteConfig, branch string, headCommit string, packs [][]byte) (*Repository, error) {
	if err := rejectLegacyRepositoryState(stateDir); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(stateDir, "config.toml")); err == nil {
		return nil, ErrRepositoryAlreadyInitialized
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect repository configuration: %w", err)
	}

	if err := ensureDurableDirectory(stateDir); err != nil {
		return nil, fmt.Errorf("create repository state directory: %w", err)
	}

	repo := newRepository()
	repo.mergeStateDir = stateDir
	repo.objectStore = newLooseObjectStore(stateDir, &repo.objects)
	repo.stateLock = flock.New(filepath.Join(stateDir, "repository.lock"))
	locked, err := repo.stateLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock merge repository: %w", err)
	}
	if !locked {
		return nil, ErrMergeRepositoryLocked
	}

	if branch == "" {
		branch = defaultBranchName
	}
	repo.defaultBranch = branch
	repo.activeBranch = branch
	repo.remote = &cfg
	if repo.remoteBranchTracking == nil {
		repo.remoteBranchTracking = make(map[string]RemoteBranchTracking)
	}

	if len(packs) == 0 {
		if err := repo.seed(); err != nil {
			return nil, closeAfterFailedOpen(repo, fmt.Errorf("seed cloned repository: %w", err))
		}
		if headCommit != "" {
			repo.remoteBranchTracking[branch] = RemoteBranchTracking{
				RemoteBranch:     branch,
				RemoteHeadCommit: headCommit,
			}
		}
	} else {
		frames, err := decodePullPackFrames(packs)
		if err != nil {
			return nil, closeAfterFailedOpen(repo, err)
		}
		if len(frames) == 0 {
			return nil, closeAfterFailedOpen(repo, fmt.Errorf("%w: clone pack list is empty", ErrPullInvalidPack))
		}
		if frames[0].Base.ID != "" {
			return nil, closeAfterFailedOpen(repo, fmt.Errorf("%w: clone pack must declare a from-scratch base", ErrPullInvalidPack))
		}
		if err := validatePullFrameChain(frames); err != nil {
			return nil, closeAfterFailedOpen(repo, err)
		}

		restore := repo.beginPullInstallLocked()
		defer func() { repo.objectBatch = nil }()

		currentLocalHead, _, _, err := repo.installPullFramesLocked(context.Background(), frames, map[string]ObjectID{}, "", headCommit)
		if err != nil {
			restore()
			return nil, closeAfterFailedOpen(repo, err)
		}

		if packErr := repo.objectBatch.publish(); packErr != nil && !packPublicationCommitted(packErr) {
			restore()
			return nil, closeAfterFailedOpen(repo, fmt.Errorf("repository: publish cloned immutable objects: %w", packErr))
		}

		repo.branches[branch] = currentLocalHead
		if headCommit != "" {
			repo.remoteBranchTracking[branch] = RemoteBranchTracking{
				RemoteBranch:     branch,
				RemoteHeadCommit: headCommit,
			}
		}
		if err := repo.ensureBranchHeadProjectionsLocked(); err != nil {
			restore()
			return nil, closeAfterFailedOpen(repo, fmt.Errorf("repository: pin cloned snapshot: %w", err))
		}
	}

	if err := repo.initializeControlStateLocked(); err != nil {
		return nil, closeAfterFailedOpen(repo, fmt.Errorf("initialize repository control state: %w", err))
	}

	if err := repo.RecoverMergeTransactions(); err != nil {
		return nil, closeAfterFailedOpen(repo, err)
	}
	if err := repo.ensureStartupProjections(); err != nil {
		return nil, closeAfterFailedOpen(repo, err)
	}

	return repo, nil
}
