# Releasing

Releases are automated from [Conventional Commits](https://www.conventionalcommits.org/)
on `main`. Nobody builds or uploads release artefacts by hand.

## Flow

1. Every push to `main` runs [`release.yml`](../.github/workflows/release.yml).
   **release-please** opens (or updates) a pull request titled `chore: release X.Y.Z`
   with the next version and the `CHANGELOG.md` entry, computed from the commit
   messages since the last tag.
2. Review the release PR (version, changelog) and squash-merge it.
3. release-please creates the `vX.Y.Z` tag and the GitHub release, and the same
   workflow then publishes:
   - **Binaries** (GoReleaser): `linux/amd64` and `linux/arm64` archives with
     `LICENSE`, `NOTICE`, `README.md`, `.env.example` and the Swarm stack example;
     `checksums.txt`; one SPDX SBOM per archive; `checksums.txt.sigstore.json`
     (keyless cosign signature); SLSA build provenance (GitHub attestation).
   - **Container image** on `ghcr.io/carloseduardo21/apisix-dashboard` for
     `linux/amd64` and `linux/arm64`. The image is scanned by Trivy (fixable
     HIGH/CRITICAL fail the release) **before** it is pushed, then signed with
     cosign, with BuildKit SBOM and SLSA provenance attestations.

### Versioning

- Semantic versioning, tags `vX.Y.Z`.
- Before 1.0: `feat` bumps the minor version, `fix` the patch version, and breaking
  changes (`!` / `BREAKING CHANGE:`) bump the minor version.
- `feat`, `fix`, `perf`, `revert`, `docs` and `build` appear in the changelog; `ci`,
  `test`, `refactor`, `style` and `chore` are hidden.
- The first release is forced to `0.1.0` by `release-as` in
  [`release-please-config.json`](../release-please-config.json). **Remove
  `release-as` after v0.1.0 is published**, otherwise every release PR proposes 0.1.0.
- Image tags: `X.Y.Z`, `X.Y`, `latest`, and `X` from 1.0 on.

### Notes

- Tags created with `GITHUB_TOKEN` do not trigger other workflows, so publishing
  happens in the same workflow, gated on `release_created`.
- Pull requests opened with `GITHUB_TOKEN` do not trigger CI either: the release PR
  has no status checks, and the maintainer merges it with the admin bypass of the
  branch protection. The release PR only changes `CHANGELOG.md` and the manifest; the code it releases already passed CI on `main`. A GitHub
  App token would make CI run on it, if that is ever needed.
- The repository setting *Allow GitHub Actions to create and approve pull requests*
  must be enabled for release-please to open its PR.
- New GHCR packages are **private** by default: after the first release, set the
  package visibility to public in the package settings.

## Local checks

```bash
make release-check      # goreleaser check
make release-snapshot   # full local build into dist/, without publishing or signing
```

## Verifying a release

Binaries:

```bash
VERSION=0.1.0
gh release download "v${VERSION}" -R carloseduardo21/apisix-dashboard
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/carloseduardo21/apisix-dashboard/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
gh attestation verify "apisix-dashboard_${VERSION}_linux_amd64.tar.gz" -R carloseduardo21/apisix-dashboard
```

Container image (pin by digest in production):

```bash
IMAGE=ghcr.io/carloseduardo21/apisix-dashboard:0.1.0
cosign verify "$IMAGE" \
  --certificate-identity "https://github.com/carloseduardo21/apisix-dashboard/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify "oci://$IMAGE" -R carloseduardo21/apisix-dashboard
```

## Tooling and why it is used

Release tooling runs only in CI (and optionally locally); none of it ships in the
binary or the image. Every action is pinned by commit SHA and every tool by exact
version; Dependabot proposes updates after a 7-day cooldown.

| Tool | Why | Why not something simpler |
|---|---|---|
| release-please (`googleapis/release-please-action`, Apache-2.0, Google) | Version, changelog and tag from Conventional Commits, reviewed through a PR | Hand-written tags and changelogs are error-prone and skip review |
| GoReleaser (`goreleaser/goreleaser`, MIT) | Reproducible cross-compiled archives, checksums, SBOM and signing in one declarative file | A shell script would reimplement archiving, checksums and asset upload |
| cosign (`sigstore/cosign`, Apache-2.0, OpenSSF) | Keyless signatures for checksums and images, tied to this workflow's identity; no key to store or leak | GPG keys need storage, rotation and distribution |
| syft (`anchore/syft`, Apache-2.0) | SPDX SBOM per archive, used by GoReleaser | No stdlib alternative; SBOMs support PCI DSS 6.3.2 (software inventory) |
| Docker actions (`setup-buildx`, `metadata`, `login`, `build-push`, Apache-2.0) | Multi-arch build, OCI labels and tags, BuildKit SBOM/provenance attestations | Already the standard way to build images on GitHub Actions |
| `actions/attest-build-provenance` (MIT, GitHub) | SLSA build provenance verifiable with `gh attestation verify` | Required for SLSA level 2+ provenance on GitHub |
| Trivy (`aquasecurity/trivy`, Apache-2.0) | Blocks publishing an image with fixable HIGH/CRITICAL vulnerabilities | Already used in CI; downloaded with a pinned checksum |
