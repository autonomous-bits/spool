---
name: spool-release
description: Prepare, publish, and verify a versioned Spool CLI release through the protected main branch. Use when asked to release Spool, create a release tag, or automate the changelog-to-GitHub-Release workflow.
---

# Spool release

Use this skill to publish a semantic-version release of the `spl` CLI. The release workflow is
defined in `.goreleaser.yaml` and `.github/workflows/release.yml`; it builds macOS, Linux, and
Windows archives and publishes a GitHub Release when a `v*` tag is pushed.

## Required safeguards

- Require a version in `vMAJOR.MINOR.PATCH` form. If the request omits it, ask for the version.
- Use `gh auth status` and `gh repo view --json nameWithOwner,viewerPermission` before changing
  GitHub resources. The authenticated user must have the rights required to push a tag.
- Never tag an unmerged branch. The tag must point to the final `origin/main` merge commit.
- Never replace, force-push, move, or delete a release tag. If the requested tag exists, stop and
  report it.
- Do not bypass `main` protection unless the requester explicitly authorizes an administrator
  bypass. Prefer the normal review and merge process.

## Release procedure

1. Fetch `origin/main`, then create a `user/<name>/prepare-vX.Y.Z` branch from it.
2. Update `CHANGELOG.md`:
   - Leave `## [Unreleased]` in place.
   - Move relevant user-facing entries into `## [X.Y.Z] - YYYY-MM-DD`.
   - Do not add technical-only release automation entries unless they are user-facing.
3. Check whitespace with `git diff --check`, commit only the changelog update, push the branch,
   and open a pull request against `main` with `gh pr create`.
4. Wait for all required checks and reviews. If auto-merge is configured, it may be enabled with
   `gh pr merge <number> --auto --squash`.
5. After the pull request merges, fetch `origin/main` and verify it is the merge commit. Create
   and push an annotated tag:

   ```sh
   git tag -a vX.Y.Z origin/main -m "Release vX.Y.Z"
   git push origin vX.Y.Z
   ```

6. Find the tag-triggered `Release` workflow with `gh run list --workflow Release --branch vX.Y.Z`.
   Wait with `gh run watch <run-id> --exit-status`.
7. Verify the published release and assets using:

   ```sh
   gh release view vX.Y.Z --json url,isDraft,isPrerelease,tagName,assets
   ```

Report the release URL, commit SHA, and asset names. If the release workflow fails, report the
failed job and do not alter the tag; diagnose and release a new version after the fix is merged.

## Repository release controls

- `main` requires a code-owner approval and successful checks. The user may be the only
  contributor; administrator bypass is available but must be explicitly requested for a
  self-reviewed release.
- The active `v*` tag ruleset restricts creation, modification, and deletion of release tags.
  The designated release maintainer has a bypass for creating a new tag.
- The release workflow uses `GITHUB_TOKEN` with `contents: write`; no release secret is required.

## Invocation examples

- `Use the /spool-release skill to prepare v0.0.2.`
- `Use /spool-release to release v1.0.0, using the administrator bypass after checks pass.`
