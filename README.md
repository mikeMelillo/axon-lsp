# Axon Language Server (axon-lsp)

The extension bundles its Go language server; no separate Python or Haxall runtime is required. Binaries are included for Linux, Windows, and macOS.

A VS Code extension that provides Language Server Protocol support for the Axon programming language used in SkySpark and Haxall.

This project supports legacy Trio/Fantom workflows alongside Haxall 4.x Xeto functions and source archives. Analysis is conservative: unknown types do not produce speculative mismatch warnings, and this is not a full Axon or Xeto compiler.

## 0.4.3 Changes

- Parse `->` as tag lookup, including mixed trap/dot/index chains, without mistaking tag names for function references.
- Require an existing binding for `=` reassignment; preserve same-line ordering, lexical scope, and conservative typing after mutation.
- Preserve call context and source positions in Trio and embedded Xeto, with improved recovery around incomplete arrows and multiline Trio `src:` fields.

See the [analysis and type reference](docs/type-intelligence.md) for validation results and known limitations.

## Features

- **Autocomplete**: Intelligent code completion for Axon functions
- **Hover Information**: View function documentation on hover
- **Go to Definition**: Navigate to local definitions and known core/external function sources
- **Signature Help**: See function arguments while typing
- **References**: Find function usages in the workspace
- **Diagnostics**: Undefined functions, unresolved receivers and simple arguments, undefined reassignment targets, missing arguments, and definite argument-type mismatches
- **Xeto Support**: Embedded Axon analysis, typed function signatures and defaults, and read-only `.xetolib` source navigation
- **Block Navigation**: Highlight and fold matching `do`/`end`, `defcomp`/`end`, `if`/`else`, and `try`/`catch` pairs
- **Document Structure**: Function ranges support Outline, Breadcrumbs, and VS Code Sticky Scroll

## Installation

### From Marketplace (recommended)

[Marketplace Install](https://marketplace.visualstudio.com/items?itemName=mikeMelillo.axon-lsp)

### From Source

```bash
# Install dependencies
npm install

# Optional: refresh bundled core cache
npm run build:cache

# Package the extension
npm run package

# Install the .vsix file
code --install-extension build/axon-lsp-<version>.vsix
```

## Requirements

- VS Code 1.80+

The language server is bundled as a platform-specific Go binary inside the extension for:

- Linux x64 / arm64
- macOS x64 / arm64
- Windows x64 / arm64

## Configuration

The extension works out of the box with its bundled function cache and workspace scanning.

Available settings:

- `axonLsp.externalPaths`: additional source directories outside the workspace, including Haxall installations or source checkouts
- `axonLsp.mode`: function-metadata preference (`auto`, `defs`, or `specs`; default: `auto`)
- `axonLsp.indexAllWorkspaceFolders`: index every folder in a multi-root VS Code workspace as local Axon sources
- `axonLsp.ignoreBundledXetoCache`: ignore bundled Xeto function metadata while retaining workspace, external-path, and archive sources, plus the legacy defs cache (default: `false`)
- `axonLsp.warnMissingArguments`: warn about omitted required arguments (default: `true`); explicit Xeto parameter defaults permit omission without changing the declared nullability

Notes:

- Configured external paths are indexed recursively up to 4 directory levels below each root for `.trio`, `.axon`, `.fan`, `.xeto`, and `.xetolib` sources.
- `axonLsp.externalPaths` can point at a Haxall-style clone root; source folders must fall within that depth budget.
- `axonLsp.haxallPaths` has been removed. Move its entries into `axonLsp.externalPaths` and remove the old setting. To preserve previous precedence, place the former Haxall paths before other external paths.
- Multi-root indexing is disabled by default, so only the first VS Code workspace folder is scanned. Enabling it may increase startup time and memory usage for large workspaces.
- If the same function exists in multiple places, requests prefer definitions in the current file, then workspace sources, then configured external roots, then the bundled core cache. Selected workspace folders form one index; there is no separate local/global workspace tier.
- Within the workspace, editable sources take precedence over `.xetolib` entries. The same rule applies within each external root. Unsaved edits update lookup, completions, hover, signature help, definitions, and diagnostics for open documents; closing a buffer restores its disk content or falls back to the remaining sources.
- Within `axonLsp.externalPaths`, earlier paths take precedence when functions have the same name. These paths do not control the bundled cache.
- Enable `axonLsp.ignoreBundledXetoCache` to exclude bundled Xeto metadata from function lookup, completions, hover, signature help, definitions, and diagnostic resolution in every mode. Changes apply without restarting the server and refresh diagnostics for open documents. Legacy bundled defs remain available as fallback; this is not a workspace-only setting and does not remove bundled assets from the extension.
- Enable VS Code's `editor.stickyScroll.enabled` setting to keep the current function declaration visible while scrolling through long bodies.

### Xetolib Archives

Workspace and external-path scans read `.xetolib` ZIP archives containing normal Xeto source trees, such as `example.lib/lib.xeto` and `example.lib/specs.xeto`. Each `.xeto` entry is indexed using the existing Xeto parser for callable functions and spec names; archives are read in memory without extracting files.

- Go to Definition opens archive entries as read-only virtual source documents.
- Archive replacement and deletion refresh the index and invalidate stale entries. Changes are watched in both workspace and configured external directories.
- Build-output directory creation/removal triggers debounced reconciliation, including configured roots that do not exist yet. Archive updates refresh caller diagnostics and use bounded recovery retries for files observed mid-write. VS Code watcher exclusions or writes completing after the retry window can still require another event or a window reload.
- Archives are source inputs only, not editable Xeto documents. Diagnostics still apply only to `.trio` and `.xeto` documents.
- Library dependency resolution and library/version selection are not implemented; archives follow the existing source-root precedence.
- Invalid archives are skipped. Each archive is limited to 128 MiB compressed input, 4,096 entries, 4 MiB of central-directory metadata, 8 MiB per decompressed entry, and 64 MiB total decompressed content. Unsafe entry paths and duplicate paths are rejected; non-Xeto resources also count toward these limits. ZIP64, multi-disk archives, and nonstandard layouts requiring ZIP offset recovery are unsupported.

### Diagnostic Semantics

Both `value.func()` and `value.func` are checked as dot calls, with the receiver bound to the first parameter. Undefined receivers are reported even when a function lacks signature metadata; declared values with unknown types are not treated as undefined.

Axon operators have distinct roles:

```axon
rec: readById(@example)       // : defines a variable
rec = readById(@replacement) // = reassigns an existing variable
rec->siteRef->dis            // -> looks up literal tag names (trap)
rec->curVal.toStr            // . calls a function with an implicit receiver
```

Trap expressions support chaining, calls, and indexing. An unresolved Axon receiver is reported, but tag names are not variables or function references. Filter paths such as `readAll(siteRef->dis == "HQ")` retain their tag-path meaning. Trap-result types remain unknown; the server does not resolve referenced records or check whether a runtime tag exists.

Reassignment does not define a variable. Definitions and assignments are resolved in source order within lexical function scopes, including same-line statements. Parameters and captured outer bindings may be reassigned; inner definitions do not escape their function. After reassignment, inferred value types are conservatively unknown rather than copied from an outdated initializer.

Xeto parameter types and defaults are indexed separately. For example, `opts: Dict? <axon:"null">` accepts explicit `null` and may be omitted, while `opts: Dict <axon:"null">` has an omission default but is not inferred to accept explicit `null`. Defaults are preserved as Axon source, not evaluated. Nullable parameters without explicit defaults retain the existing omission behavior. Date, Time, DateTime, and DateSpan literals are distinguished from numbers.

Metadata not preserved by older bundled catalogs remains incomplete until those catalogs are regenerated. Structural Xeto conformance, complete syntax-error reporting, and flow-sensitive variable type inference remain outside the current diagnostic scope. Some recovery and editor paths still use line-oriented scanning.

## How It Works

Axon LSP currently sources functions from:

- workspace sources and unsaved edits
- configured external source directories
- `.xetolib` archives within those roots
- bundled legacy defs and Haxall 4.0.6 Xeto metadata, including selected SkySpark/core metadata

The current parser/indexer supports:

- `func` records in `.trio`
- `@Axon` definitions in `.fan`
- `defcomp` functions
- Xeto `Func` signatures, parameter defaults, and embedded Axon bodies
- Source-backed `.xetolib` archives and Xeto spec names (not full structural conformance)

The extension keeps a compact cached backstop rather than shipping the full Haxall source tree, which keeps installs smaller and startup more predictable.

## Understanding the Project

Axon is the language this extension analyzes, but it is usually written inside
another file format rather than opened as a standalone `.axon` document. Axon
source commonly appears in `.trio` function records and in embedded bodies in
`.xeto` files. `.fan` files are useful as a source of `@Axon` function
definitions and metadata.

The file types play different roles:

- **Axon** is the expression language: function calls, local variables,
  literals, lambdas, and related expressions.
- **Trio** is a host format for records, including function records whose
  `src` field can contain Axon.
- **Xeto** describes Specs and functions, including parameter and return types;
  a Xeto `Func` can also contain an embedded Axon body.
- **Fantom** source can provide `@Axon` function definitions for indexing.

When Axon is embedded, the server extracts the Axon region, analyzes it, and
maps editor features back to positions in the host document. Syntax coloring
comes from the TextMate grammars in `syntaxes/`; parsing, symbol lookup, and
diagnostics are handled by the Go server.

### Client and Server

The VS Code client is a small TypeScript layer that starts the language server
and connects it to VS Code using the Language Server Protocol (LSP). Most
language behavior lives in the Go server:

```text
VS Code  ⇄  client/src/extension.ts  ⇄  Go language server
                                          ├─ parse and extract source
                                          ├─ index and resolve symbols
                                          └─ answer LSP requests
```

The main developer entry points are:

| Path | Responsibility |
| --- | --- |
| `client/src/extension.ts` | Extension activation, configuration, and server startup |
| `go-server/cmd/axon-lsp/` | Go language-server executable |
| `go-server/internal/lsp/` | LSP request handling and coordination |
| `go-server/internal/axon/` | Axon tokens, expression AST, source spans, and call context |
| `go-server/internal/diag/` | Semantic diagnostics and call-argument validation |
| `go-server/internal/index/` | Function, Spec, definition, and reference indexes |
| `go-server/internal/scope/` | Local-variable and parameter scope analysis |
| `go-server/internal/trio/` | Trio parsing and embedded Axon extraction |
| `go-server/internal/xeto/` | Xeto parsing, function metadata, and embedded Axon extraction |
| `go-server/internal/fantom/` | Fantom `@Axon` source indexing |
| `go-server/internal/cache/` | Loading bundled function metadata |
| `syntaxes/` | TextMate grammars for editor coloring |
| `scripts/` | Build and cache-generation scripts |

### How an Editor Request Is Resolved

For example, when the server analyzes an embedded Axon function in a Xeto
document, the work is roughly:

1. The LSP layer identifies the Xeto function and its embedded Axon body.
2. The Axon lexer and parser produce tokens and source-spanned expressions.
3. Scope analysis distinguishes local bindings from global names.
4. The index supplies known functions, Specs, signatures, and source locations.
5. Diagnostics or editor results are mapped back to the original Xeto
   document.

Trio has a corresponding extraction path for Axon in `src` fields. Function
definitions and metadata may come from the workspace, configured source roots,
or the bundled cache; the configured precedence is described above.

Some existing analysis still uses line-oriented scanning alongside the Axon
AST. The AST is used for structured expression and call analysis, while older
scanning helpers remain in parts of the validation and reference paths. For
feature-specific implementation details, start in `internal/lsp/` and follow
the call into the responsible package.

### Local Debugging

Run the server checks from `go-server/` with `go test ./...` and `go vet ./...`. From the repository root, `npm run test:client` checks watcher behavior and `npm run compile` checks the TypeScript client.

Press <kbd>F5</kbd>, or pick the `Run Extension` configuration in `.vscode/launch.json`. Its `build-dev` pre-launch task runs two steps in sequence:

| Task | Script | What it does |
| --- | --- | --- |
| `build-server-dev` | `npm run build:server:dev` | Builds **only** the Go binary this machine can run |
| `watch-client` | `npm run watch` | `tsc -watch`; also the default build task, so <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>B</kbd> recompiles as you edit |

`build:server:dev` passes `--host` to `scripts/build-go.mjs`, which skips the five cross-compiled targets you can't run locally. The saving is small once Go's build cache is warm, but the first build after a reboot is where it pays:

| | All 6 targets | Host only |
| --- | --- | --- |
| Warm cache | ~1.1s | ~0.7s |
| Cold cache | ~44s | ~8s |

Keep using `build:server` (all six) for packaging: `build:all`, `package`, and `publish` all depend on it, and the VSIX has to ship a binary for every platform.

The manifest pins this extension to the `workspace` host so its Go server runs beside the workspace (including over SSH). The debug launch opens `test/file.trio` and the extension starts its language client immediately in Development mode, even if the host restores another editor. Outside Development mode, the extension stays lazy: it starts the client when the first Axon, Xeto, or Fantom document is opened. The `Axon Language Server` output channel should therefore appear immediately in the debug host; its `Using server binary:` line should point into this checkout's `bin/` directory.

You can override the binary used at runtime with `AXON_LSP_SERVER_PATH` if you want to point the extension at a custom local build.

#### If your edits seem to do nothing

The Extension Development Host will happily run an **installed** copy of `mikeMelillo.axon-lsp` instead of your working tree — a VSIX you side-loaded, or the published build, since both share the extension ID. The extension still activates and serves requests, so it looks healthy, but it is running a frozen snapshot and none of your edits reach it.

Check the `Axon Language Server` output channel and read the second line:

```text
Axon Extension Activating...
Using server binary: /path/to/axon-lsp/bin/axon-lsp-linux-x64
```

If that path points inside `~/.vscode-server/extensions/` (remote) or `~/.vscode/extensions/` (local), the dev host is not running your code. Disable the installed copy — Extensions view → `axon-lsp` → gear → **Disable** — or uninstall it, then reload the window. Clients that sync extensions can reinstall it, so re-check this if the symptom comes back.

## Roadmap

See [ROADMAP.md](./ROADMAP.md) for product direction and the [planning index](docs/plans/README.md) for implementation proposals.

## License

MIT License - see [LICENSE](./LICENSE) file.

## Note

Earlier versions of this project used a Python `pygls` server as a quick way to get the extension off the ground. The supported runtime is now the bundled Go server.
