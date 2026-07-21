# Release and Distribution

Multigent's first public distribution model follows a CLI-first product shape:

- GitHub Releases are the source of truth for versioned native binaries.
- Install scripts and Homebrew are the primary human-friendly install channels.
- npm is a thin wrapper that downloads the matching native binary.
- Docker images are published for self-hosted demos and agent runtime sandboxes.

## Release Artifacts

Every release tag `vX.Y.Z` publishes one archive per platform:

```text
multigent-vX.Y.Z-linux-amd64.tar.gz
multigent-vX.Y.Z-linux-arm64.tar.gz
multigent-vX.Y.Z-darwin-amd64.tar.gz
multigent-vX.Y.Z-darwin-arm64.tar.gz
multigent-vX.Y.Z-windows-amd64.zip
multigent-vX.Y.Z-windows-arm64.zip
checksums.txt
```

Each archive contains:

- `multigent`: human/admin CLI and self-hosted web server.
- `mga`: scoped runtime CLI mounted into agent sandboxes.

`mga` must be released with `multigent`; otherwise Docker sandbox runs cannot reliably report tasks, read docs, or complete workflow steps.

## Install Channels

Recommended install:

```bash
curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | bash
```

Windows:

```powershell
irm https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.ps1 | iex
```

Homebrew:

```bash
brew install multigent/tap/multigent
```

npm:

```bash
npm install -g @multigent/multigent
```

The npm package must keep `npm/package.json` version equal to the release tag without the leading `v`; the release workflow fails if they drift.

## Docker Images

The release workflow publishes:

```text
ghcr.io/multigent/multigent:latest
ghcr.io/multigent/multigent/runtime-base:latest
ghcr.io/multigent/multigent/sandbox-codex:latest
ghcr.io/multigent/multigent/sandbox-claudecode:latest
ghcr.io/multigent/multigent/sandbox-gemini:latest
ghcr.io/multigent/multigent/sandbox-generic:latest
```

The critical image for first-run agent execution is:

```text
ghcr.io/multigent/multigent/runtime-base:latest
```

It must remain public before announcing a release; otherwise new users will fail on their first Docker sandbox run. Both runtime-image workflows log out of GHCR and verify anonymous manifest access after publishing, so a private package fails CI instead of shipping a broken installation path. GitHub defaults newly created GHCR packages to private; set each new package to **Public** in the organization package settings once.

## Release Steps

1. Update `npm/package.json` to the target version.
2. Update release notes.
3. Commit the version changes.
4. Tag:

   ```bash
   git tag vX.Y.Z
   git push origin main --tags
   ```

5. Wait for `.github/workflows/release.yml`.
6. Confirm GitHub Release assets, GHCR packages, and npm package if `NPM_TOKEN` is configured.
7. Confirm the public quickstart:

   ```bash
   curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | bash
   multigent version
   mga version
   ```

## Homebrew Tap

The release workflow updates `multigent/homebrew-tap` when `HOMEBREW_TAP_GITHUB_TOKEN` is configured. If the token is absent, the release still succeeds because the install script falls back to GitHub Releases binaries.

The tap formula installs both binaries:

```ruby
bin.install "multigent"
bin.install "mga"
```

Homebrew should be treated as the polished install channel, not the only install channel.
