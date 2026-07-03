# Releasing

Releases are tag-driven. Pushing a `v*` tag runs
[`.github/workflows/release.yml`](../.github/workflows/release.yml), which has
two independent jobs:

- **goreleaser** — verifies the schema pin, runs the tests, then builds the
  cross-platform binaries, archives, checksums, and the GitHub Release page.
- **image** — builds and pushes the multi-arch container image to
  `ghcr.io/piotrsenkow/mlsgrid-mcp`. It is independent of goreleaser, so a
  registry hiccup never blocks the binary release.

## Pre-flight

- [ ] On `main`, working tree clean, CI green.
- [ ] `docs/tools.md` and the `tools/list` golden reflect the current tools.
- [ ] `make verify-pin` passes (the schema pin matches upstream).
- [ ] `server.json` `version` matches the tag you're about to cut (see below).

## Cut the release

```sh
git tag -a v0.1.0 -m "mlsgrid-mcp v0.1.0"
git push origin v0.1.0
```

Watch it: `gh run watch` (or the Actions tab).

> **Test the pipeline first (optional):** push `v0.1.0-rc.1` instead. goreleaser
> treats `-rc` / `-beta` tags as pre-releases, so it exercises the whole flow
> without spending the real version number.

## Verify

- **Releases page** shows the tag with binaries + `checksums.txt`, a changelog,
  and the compliance footer.
- **Install works:**
  ```sh
  go install github.com/piotrsenkow/mlsgrid-mcp/cmd/mlsgrid-mcp@v0.1.0
  mlsgrid-mcp version   # prints v0.1.0
  ```
- **Image works:**
  ```sh
  docker run --rm ghcr.io/piotrsenkow/mlsgrid-mcp:0.1.0 version
  ```
  **First release only:** the ghcr package is created private. Make it public in
  the repo's *Packages* settings so the registry and users can pull it.

## Rollback

Nothing depends on a fresh tag, so a bad release is cheap to undo:

```sh
gh release delete v0.1.0 --cleanup-tag --yes   # removes the release and the tag
```

Fix, then re-tag.

## Promotion (do when ready — not required for a usable release)

Tagging makes binaries and an image exist; these steps put the server in front
of people.

### MCP registry

The manifest is [`server.json`](../server.json) (an `oci` package pointing at the
image above). Publish with the official
[`mcp-publisher`](https://github.com/modelcontextprotocol/registry) CLI. The
namespace `io.github.piotrsenkow/*` is authorized by owning the GitHub account.

Locally (interactive GitHub device-flow login):

```sh
mcp-publisher login github
mcp-publisher publish        # reads ./server.json
```

Bump `server.json`'s `version` (and the package `version`) to match each release
before publishing.

### awesome-mcp-servers

Open a PR against [`punkpeye/awesome-mcp-servers`](https://github.com/punkpeye/awesome-mcp-servers)
(and/or `wong2/awesome-mcp-servers`), matching that list's category and icon
legend. Suggested entry:

```markdown
- [piotrsenkow/mlsgrid-mcp](https://github.com/piotrsenkow/mlsgrid-mcp) 🏎️ 🏠 - Real-estate (RESO / MLS Grid) data over MCP: search listings, comps, market stats, price history, and open houses over a Postgres database populated by mlsgrid-sync.
```

## Cutting a later release

1. Land the changes on `main` (CI green).
2. If the vendored mlsgrid-sync migration changed, refresh the pin
   (`adapters/postgres/testdata/contract/PIN`) and bump
   `ExpectedContractMajor` only on a *major* contract change.
3. Bump `server.json` `version` to the new tag.
4. Tag and push as above; re-publish to the registry if you promote.
