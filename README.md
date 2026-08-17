# Axon Language Server (axon-lsp)

> **POC Notice**: This extension was built with AI assistance and is still evolving quickly. Use it, kick the tires, and expect a few rough edges along the way.

A VS Code extension that provides Language Server Protocol support for the Axon programming language used in SkySpark and Haxall.

This project is still very much a practical tool built in the open. The near-term goal is to make Axon development in VS Code feel lightweight and dependable around the Haxall 3.1.12 era, while laying the groundwork for the 4.x/Xeto shift that is coming next.

## Features

- **Autocomplete**: Intelligent code completion for Axon functions
- **Hover Information**: View function documentation on hover
- **Go to Definition**: Navigate to local definitions and known core/external function sources
- **Signature Help**: See function arguments while typing
- **References**: Find function usages in the workspace
- **Diagnostics**: Real-time error detection for undefined functions

## Installation

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

### From Marketplace

[Marketplace Install](https://marketplace.visualstudio.com/items?itemName=mikeMelillo.axon-lsp)

## Requirements

- VS Code 1.80+
- No external Python or pip dependencies required
    - Python server removed as part of v0.2.0
    - This causes the extension static size to be much larger (~700kB -> 13MB), but the server no longer has external install dependencies

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

## Building

```bash
# Refresh the bundled function cache
npm run build:cache

# Compile the extension and all Go server binaries
npm run build:all

# Package a full VSIX in ./build
npm run package
```

### Local Debugging

- `Run Extension` in `.vscode/launch.json` builds the Go server first, then starts the TypeScript watch task.
- You can override the binary used at runtime with `AXON_LSP_SERVER_PATH` if you want to point the extension at a custom local build.

## Roadmap

See [ROADMAP.md](./ROADMAP.md) for the current plan.

## License

MIT License - see [LICENSE](./LICENSE) file.

## Note

Earlier versions of this project used a Python `pygls` server as a quick way to get the extension off the ground. The supported runtime is now the bundled Go server.
