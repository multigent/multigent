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
- `mga`: scoped runtime CLI synchronized into agent sandboxes.

`mga` must be released with `multigent`; otherwise Docker sandbox runs cannot reliably report tasks, read docs, or complete workflow steps. Runtime images include a fallback `mga`, but normal sandbox startup downloads and caches the server-matching `mga` release asset in the persistent toolchain volume.

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

The npm package must keep `npm/package.json` version equal to the release tag without the leading `v`; the release workflow fails if they drift. npm publishing is a local maintainer step, not a GitHub Actions job.

## Docker Images

The release workflow publishes:

```text
ghcr.io/multigent/multigent:latest
ghcr.io/multigent/multigent/runtime-base:latest
```

The critical image for first-run agent execution is:

```text
ghcr.io/multigent/multigent/runtime-base:latest
```

It must remain public before announcing a release; otherwise new users will fail on their first Docker sandbox run. The runtime-image workflow logs out of GHCR and verifies anonymous manifest access after publishing, so a private package fails CI instead of shipping a broken installation path. GitHub defaults newly created GHCR packages to private; set `multigent` and `runtime-base` to **Public** in the organization package settings once.

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
6. Publish npm locally with the maintainer npm token. Do not rely on `/root/.npmrc`; it may contain an older token or a token that only covers some package scopes. The recommended path is to inject the publishing token through `shrike`:

   ```bash
   shrike exec --with=NPM_TOKEN -- sh -lc \
     'cd /root/code/spaceship/multigent/npm && npm publish --access public --//registry.npmjs.org/:_authToken="$NPM_TOKEN"'
   ```

   If npm prints `Your package is being processed and may take a few minutes to become available.`, the version has been staged. `npm view @multigent/multigent@X.Y.Z` may still return 404 briefly. Wait 1-3 minutes before treating it as a failure. If a retry returns `Cannot publish over previously staged version "X.Y.Z"`, the first publish was accepted; keep waiting for registry convergence.

7. Confirm GitHub Release assets, GHCR packages, and npm package:

   ```bash
   gh release view vX.Y.Z --repo multigent/multigent --json tagName,isDraft,isPrerelease,url,assets
   npm view @multigent/multigent version dist-tags.latest --json
   npm view @multigent/multigent@X.Y.Z version dist.tarball --json
   ```

8. Confirm the public quickstart:

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
