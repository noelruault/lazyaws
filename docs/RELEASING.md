# Releasing

Three channels ship every release, and they must agree: the **git tag**, the **Go module** (`proxy.golang.org`), and the **Homebrew tap** (`noelruault/homebrew-tap`). CI does not release anything — `.github/workflows/ci.yml` only runs `make prepare-release` on pushes to `main` and on PRs. Releases are driven by hand, in the order below.

Read this whole file before tagging. A published version cannot be unpublished (see [What you cannot undo](#what-you-cannot-undo)); the only remedy for a bad release is another release.

## Preconditions

- `make setup` once per clone, so `.githooks/pre-push` runs `make prepare-release` before every push.
- Clean working tree. `VERSION := $(shell git describe --tags --always --dirty)` in the Makefile, so a dirty tree stamps `vX.Y.Z-dirty` into the binary and into every archive name. `git describe` ignores untracked files, so only tracked modifications matter.
- The tag must exist before you build: `VERSION` comes from `git describe`, not from an argument.
- `gh` authenticated with push rights on `noelruault/lazyaws` and `noelruault/homebrew-tap`.
- If a loop or another session is working in the main worktree, build from `git worktree add <dir> <tag>` instead of checking the tag out in place.

## The sequence

```sh
# 1. tag the exact commit that will ship, and push it
git tag v0.5.0
git push origin v0.5.0                     # pre-push runs the full gate: lint, vuln, licenses, tidy -diff, race tests

# 2. build every platform archive plus the checksums file
make release-all                           # gate once on the host, then darwin/{arm64,amd64}, linux/{amd64,arm64}, windows/amd64 into dist/, then dist/SHA256SUMS

# 3. publish the GitHub release with the archives and their checksums
gh release create v0.5.0 dist/lazyaws-v0.5.0-*.tar.gz dist/SHA256SUMS --title v0.5.0 --notes-file <notes>

# 4. generate the Homebrew formula FROM the published release, then push it to the tap
make brew-formula VERSION=v0.5.0           # writes dist/lazyaws.rb
# copy dist/lazyaws.rb over Formula/lazyaws.rb in the noelruault/homebrew-tap clone, commit, push

# 5. prime the module mirror so `go install ...@latest` resolves the new version
curl -s https://proxy.golang.org/github.com/noelruault/lazyaws/@v/v0.5.0.info
```

Two ordering constraints, both load-bearing:

- **Tag before build.** `release-all` derives archive names and the `-X main.version` stamp from `git describe`. Building first produces archives whose names and `-version` output disagree with the release.
- **Release before formula.** `scripts/brew-formula.sh` prefers the *published* release's `SHA256SUMS` (it downloads them with `gh release download`) and only falls back to a local `dist/SHA256SUMS`. Generating the formula first means the tap's hashes were never checked against what people actually download.

The tap formula covers macOS and Linux on arm64 and amd64. `windows-amd64` ships as a release archive only; `brew-formula` does not read its hash.

Release notes claim only what the tagged commit verifiably contains. Before writing them, confirm the work is really in the tag: `git merge-base --is-ancestor <sha> v0.5.0`. A past release shipped notes describing unpushed local work, and those notes still describe content that landed one version later.

## Verification

Each channel has one check that proves it, and none of them is "the command exited 0":

```sh
# git tag
git ls-remote --tags origin | grep v0.5.0

# GitHub release: 5 archives + SHA256SUMS, not a draft
gh release view v0.5.0 --repo noelruault/lazyaws --json isDraft,assets \
  --jq '(.isDraft|tostring), (.assets[].name)'

# Go module: run this from OUTSIDE this repo, in a throwaway module
go list -m github.com/noelruault/lazyaws@latest        # must print v0.5.0
go list -m -versions github.com/noelruault/lazyaws     # retracted versions are absent here

# Homebrew tap: the formula's hashes must byte-match the release's own SHA256SUMS
curl -sL https://github.com/noelruault/lazyaws/releases/download/v0.5.0/SHA256SUMS
gh api repos/noelruault/homebrew-tap/contents/Formula/lazyaws.rb --jq .content | base64 -d | grep sha256
```

`https://proxy.golang.org/<module>/@v/@latest` is **not** a reliable check. That endpoint is a cached mirror value and lags behind by minutes to hours; after v0.4.0 was fully published it still reported v0.3.0. The `go` command resolves `@latest` from `@v/list` plus retraction filtering, which is why `go list -m ...@latest` is the authoritative answer. Use `@v/<tag>.info` to confirm the mirror has a specific version.

## What you cannot undo

Publishing a Go module version is permanent:

- `proxy.golang.org` never removes versions. Go team, closing exactly this request: *"To avoid breaking builds, `proxy.golang.org` doesn't remove versions. Please use retractions instead."* ([golang/go#49056](https://github.com/golang/go/issues/49056), same outcome in [#46440](https://github.com/golang/go/issues/46440)).
- Deleting the tag upstream changes nothing. The mirror keeps serving the cached version, *"even if it is not available at the origin"* ([proxy.golang.org](https://proxy.golang.org/)) — the same applies to deleting the whole repository. This was observed here: v0.1.0 through v0.3.0 stayed downloadable from the mirror after their tags were deleted from GitHub.
- `sum.golang.org` is a transparent log whose *"server never removes any log record"* ([sumdb design](https://go.googlesource.com/proposal/+/master/design/25530-sumdb.md)). Every published version's hash is public forever.
- Anything secret-shaped that reaches a tagged commit is compromised. Rotate it; the blob and its hash stay reachable.

Agent sessions additionally cannot repair a mistake by deletion: the PreToolUse guard blocks `gh repo delete`, `gh release delete`, `gh repo archive`, force-pushes, remote tag deletion, and recursive `rm` of a git repository. A human runs any of those, deliberately, outside the session.

## Retracting a bad version

`retract` is the only supported way to withdraw a version, and it needs a *new, higher* release to carry it:

1. Add the directive to `go.mod` with a rationale comment above it. The comment is the text users see, so make it useful:

   ```
   // Superseded by v0.5.0.
   retract (
       v0.4.0
   )
   ```

2. Commit, then tag and publish the new version exactly as above. The retraction only takes effect once `@latest` resolves to the version that carries it, which means the new tag must be higher than every existing release and pre-release.
3. Verify from a throwaway module outside the repo:

   ```sh
   go list -m -versions github.com/noelruault/lazyaws               # retracted versions gone
   go list -m -retracted -f '{{.Version}} {{.Retracted}}' github.com/noelruault/lazyaws@v0.4.0
   ```

Retracted versions stay downloadable on purpose, so existing builds keep working. What changes is that `go get`, `go mod tidy`, `go install @latest` and pkg.go.dev stop offering them, and `go list -m -u` warns anyone still pinned to one.

Worked precedent: v0.4.0 (2026-09-02) retracts v0.1.0 through v0.3.0 with the rationale `Superseded by v0.4.0.`, after tag deletion had already been tried and did nothing.
