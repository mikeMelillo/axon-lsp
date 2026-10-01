# Axon Language Server (axon-lsp)

> **0.3.5 Update**: The extension has gone through some serious updates from 0.1->0.3, namely a complete rewrite in Golang. The extension no longer has any external dependencies (python, libs, etc.). It has been primarily developed and tested on Linux & Windows, but the binaries are also available for MacOS

A VS Code extension that provides Language Server Protocol support for the Axon programming language used in SkySpark and Haxall.

This project is very much a practical tool built in the open. The near-term goal is to make Axon development in VS Code feel lightweight and dependable around the Haxall 3.1.12 era, while laying the groundwork for the 4.x/Xeto shift.

## Features

- **Autocomplete**: Intelligent code completion for Axon functions
- **Hover Information**: View function documentation on hover
- **Go to Definition**: Navigate to local definitions and known core/external function sources
- **Signature Help**: See function arguments while typing
- **References**: Find function usages in the workspace
- **Diagnostics**: Real-time error detection for undefined functions
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

- `axonLsp.haxallPaths`: paths to Haxall installations or source trees
- `axonLsp.externalPaths`: additional directories to scan for Axon functions
- `axonLsp.indexAllWorkspaceFolders`: index every folder in a multi-root VS Code workspace as local Axon sources

Notes:

- Configured Haxall and external paths are indexed recursively up to 4 directory levels below each root.
- `axonLsp.haxallPaths` can point at a Haxall-style clone root; the server will walk down into likely source folders as long as they fall within that depth budget.
- Multi-root indexing is disabled by default, so only the first VS Code workspace folder is scanned. Enabling it may increase startup time and memory usage for large workspaces.
- If the same function exists in multiple places, the extension prefers workspace definitions first, then configured extra roots, then the bundled core cache.
- Enable VS Code's `editor.stickyScroll.enabled` setting to keep the current function declaration visible while scrolling through long bodies.

## How It Works

Axon LSP currently sources functions from:

- your local working directory
- the open source Haxall repo cache bundled with the extension
- known SkySpark/core function metadata captured in the cache (this is a convenience to support some built in @noDoc functions)

The current parser/indexer supports:

- Haxall 3.1.12-oriented workflows today
- `func` records in `.trio`
- `@Axon` definitions in `.fan`
- `defcomp` functions

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

See [ROADMAP.md](./ROADMAP.md) for the current plan.

## License

MIT License - see [LICENSE](./LICENSE) file.

## Note

Earlier versions of this project used a Python `pygls` server as a quick way to get the extension off the ground. The supported runtime is now the bundled Go server.
