# Releasing

For maintainers. A release is a Git tag of a module: once the tag is on GitHub, `go get` and
pkg.go.dev see the version. The root module, `backend/redis`, `codec/msgpack` and `compress/zstd`
have tags of their own (`v0.7.0`, `backend/redis/v0.5.1`).

## 1. Choose the versions

Release the modules that changed since their last tag; `gh release list` shows the last tags.
While a module is at `0.x`, raise the minor version when its API or behavior changes and the patch
version for fixes only.

When another module needs the new version of the root, release the root first, then raise the
version that module requires in its `go.mod` in a pull request of its own, and release the module
after it: a `go.mod` cannot require a version that has no tag yet.

## 2. The release pull request

In `CHANGELOG.md`, the `Unreleased` section becomes the version, a new empty `Unreleased` goes on
top, and the links at the bottom move on:

```diff
 ## [Unreleased]

+## [0.7.0] - 2026-12-05
+
+Released as `v0.7.0` and `backend/redis/v0.5.1`.
+
 ### Added

 - `Policy.Jitter`: ...

 ## [0.6.0] - 2026-11-07
 ...
-[Unreleased]: https://github.com/eugeneliukindev/gashe/compare/v0.6.0...HEAD
+[Unreleased]: https://github.com/eugeneliukindev/gashe/compare/v0.7.0...HEAD
+[0.7.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.7.0
```

Title it `docs(changelog): 0.7.0` and merge it once CI passes.

## 3. Check main

```bash
git switch main && git pull
just check-all
```

## 4. Draft the releases

```bash
just gh-release v0.7.0                # notes: the 0.7.0 section of CHANGELOG.md
just gh-release backend/redis/v0.5.1  # notes: the pull requests since backend/redis/v0.5.0
```

`gh-release` creates drafts on `main` and changes nothing in the repository. No tag exists yet.

## 5. Publish

On GitHub, under Releases, read each draft and publish it, the root first. Publishing creates the
tag on `main`. The release workflow then asks the Go proxy for the version, so pkg.go.dev lists
it. The site needs nothing: every push to `main` deploys it, the release pull request included.

## 6. Check the result

```bash
gh release list --limit 4
gh run list --workflow release.yml --limit 4
go list -m github.com/eugeneliukindev/gashe@v0.7.0
```

The site showed the new version in its release notes since the release pull request.

## When something goes wrong

- **The site did not update.** Run the deploy-docs workflow again: `gh workflow run
  deploy-docs.yml --ref main`.
- **The release workflow did not run.** On a release, GitHub runs the workflow of the tagged
  commit, so a tag on a commit with an older workflow runs that one; `just version-fetch <tag>`
  does its work.
- **The notes of a release are wrong.** Edit the release on GitHub; the tag stays.
- **A released version is broken.** Release a new patch version. Never delete or move a tag: the
  Go proxy keeps every version it has seen, and the ruleset of release tags forbids it.
- **Tags pushed by hand.** Push at most three tags at once: GitHub starts no workflow for a push of
  more tags. Publishing releases one by one avoids this.
