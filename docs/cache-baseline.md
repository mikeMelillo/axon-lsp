# Bundled Cache Baseline

The bundled catalogs are generated from independent upstream repositories, not
from a Haxall installation or the developer's configured `externalPaths`.
Normal extension use and Go builds/tests consume committed assets; only cache
regeneration requires the source checkouts and Git.

## Locked inputs

[`cache_sources/baseline.lock.json`](../cache_sources/baseline.lock.json) is the
authoritative source list. It records checkout paths, full commits, declared
versions, portable source namespaces, license files, build properties, and
required baseline files.

| Input | Checkout under `cache_sources/` | Revision / provenance |
| --- | --- | --- |
| Curated legacy defs | `coreFuncs.trio` | Content SHA-256 recorded on generation |
| Haxall 3.1.12 | `haxall-3.1.12/` | `ec4ab0bae96ed840d0888be2164fe672cdee8781` |
| Haxall 4.0.6 | `haxall-4.0.6/` | `b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f` |
| Project Haystack Xeto 5.0.0 | `xeto/` | `deb813507b8255d2159db81c5caf9b0ef0ad102e` |

The upstream repositories are [Haxall](https://github.com/haxall/haxall) and
[Project-Haystack/xeto](https://github.com/Project-Haystack/xeto). Clone them into
the listed paths and check out the exact locked revisions. The clones are
ignored by this repository and excluded from the VSIX; do not add them as nested
repositories/submodules merely to regenerate assets.

Xeto provides the separate `sys`, `ph`, and related standard libraries. These
do not live in Haxall's source tree. Its current declared `ph.version` is
`5.0.0` with `ph.maturity=alpha`; those versions are independent of Haxall's
`4.0.6` label. Pin commits rather than relying on moving branches or labels.

## Regeneration and verification

From the repository root:

```bash
npm run build:cache
npm run check:cache
```

The first command regenerates assets; the second compares generated content
without writing files. Both verify clean, dedicated Git checkouts at the locked
commits, required tracked files, and declared build-version properties. Missing,
modified, or mismatched inputs fail before output writing begins. There is no
automatic fetch, checkout/reset, build execution, or runtime installation.

Source bytes must also match the tracked Git blobs, not merely pass Git's
clean-status check. This prevents CRLF/smudge filters from producing different
bundles under the same commit. Use unfiltered checkouts for generation (for
example, `git -c core.autocrlf=false clone ...`). The builder never rewrites an
existing source checkout to fix its line endings.

Only tracked source files are collected. Read errors are fatal rather than
silently producing a reduced catalog. Existing test-library exclusions remain
in place; Xeto collection is restricted to `src/xeto/`, excluding conversion
templates elsewhere in the repositories. Sources and metadata use
repository-specific namespaces, so standard libraries cannot be mislabeled as
Haxall sources or contain local absolute paths.

The generator reads all inputs before publishing outputs. Each asset is
replaced via a temporary file in the same directory; the checksum manifest is
written last. This is not a multi-file filesystem transaction: an interrupted
write can leave a mismatched set, which `check:cache` and embedded integrity tests
detect. Rerun generation to repair it.

## Generated assets

Files live in `go-server/internal/cache/assets/`:

- `function_cache.json`: function variants with types/defaults and provenance.
  The current baseline has 2,026 variants across 940 function names, including
  21 variants from the standard-library repository. Existing Haxall selection
  precedence and legacy fallback are retained. Regeneration also applies existing
  parser fixes to four stale signatures (`fileMountDelete`, `sessionLogout`,
  `readByIds`, and `readByIdsStream`).
- `xeto_sources.json`: verbatim source documents, license texts, and build
  property files. The current bundle has 237 documents; Xeto contributes 22
  library manifests. Examples are served at
  `axon-ext:/xeto/5.0.0/sys/types.xeto` and
  `axon-ext:/haxall/4.0.6/axon/funcs.xeto`.
- `coreFuncs.trio`: the curated legacy source backstop.
- `cache_manifest.json`: schema version, source commits/versions, library
  inventory, per-source literal build properties, and SHA-256 checksums for the
  three content assets. Also served at `axon-ext:/cache_manifest.json`.

Build properties are preserved as text and literal key/value metadata; the
generator does not execute or fully interpret `BuildVar` expressions. Haxall's
`build.props` and Xeto's `xeto-build.props` remain associated with their own
sources. License/copyright notices in source text are retained; see
[third-party notices](../THIRD_PARTY_NOTICES.md).

## Scope and future updates

This is a reproducible **editor source/metadata baseline**, not a claim that a
complete Haxall runtime was compiled or that every Xeto dependency, constraint,
or version combination was checked. Source presence and provenance are verified;
the inheritance engine, qualified namespace resolution, and structural checks
remain separate work. Raw standard-library definitions are now available for
that work, but recaching alone does not make type inheritance affect hints.

A source-level sanity check of the pinned modern Haxall/Xeto pair found no
missing explicitly named manifest dependencies, unresolved manifest `BuildVar`
keys with the combined `hx.*`/`ph.*` properties, or simple dependency/version
property mismatches. All simple, unqualified Haxall function signature type names
also occur in the combined source declarations. This inventory is not a Xeto
compiler or runtime compatibility test; qualified resolution and constraints
remain unverified.

To update a baseline, intentionally update the lock's commit/version and any
property/required-file paths, prepare clean checkouts at those revisions, then
regenerate all assets and review their diffs. Run `npm run check:cache`,
`go test ./...`, and `go vet ./...` from their appropriate directories before
committing the lock and generated assets together. Upstream licensing notices
must remain available when adding or changing sources.

The lock replaces the former hard-coded source roots and `GITHUB_BASE_3` /
`GITHUB_BASE_4` URL overrides; source links now derive from the verified revisions.
