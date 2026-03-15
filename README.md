# Axon Language Server (axon-lsp)

> **⚠️ POC Notice**: This extension was built with AI assistance (Gemini + MiniMax) as a proof-of-concept. Use at your own risk. Features and functionality may change.

A VS Code extension that provides Language Server Protocol (LSP) support for the Axon programming language used in SkySpark and Haxall.

## Features

- **Autocomplete**: Intelligent code completion for Axon functions
- **Hover Information**: View function documentation on hover
- **Go to Definition**: Navigate to function definitions
- **Diagnostics**: Real-time error detection for undefined functions

## Installation

### From Source
```bash
# Install dependencies
npm install

# Package the extension
npx vsce package

# Install the .vsix file
code --install-extension axon-lsp-{version}.vsix
```

### From Marketplace
(TBD - Coming soon)

## Configuration

The following settings can be configured in VS Code:

| Setting | Description | Default |
|---------|-------------|---------|
| `axonLsp.haxallPath` | Path to Haxall installation | Built-in defaults |
| `axonLsp.externalPaths` | Additional paths to scan for Axon functions | None |

## How It Works

The Axon LSP indexes functions from multiple sources:

1. **Local Workspace**: Scans `.trio` and `.axon` files in your open workspace
2. **External Libraries**: Scans configured library paths (e.g., custom extensions)
3. **Core Functions**: Uses built-in backstop of core SkySpark Axon functions

Priority: Local > External > Core

> **Note**: The core function backstop is derived from exported SkySpark function definitions and may lack complete type hints.

## Roadmap

See [ROADMAP.md](./ROADMAP.md) for features under development.

### Planned Features
- Signature help (parameter hints)
- Find references
- Rename symbol
- Multiple reference directories support (local Haxall repo, cached core functions, active working directory)

## Requirements

- VS Code 1.80+
- Python 3.8+

### Python Dependencies

This extension requires Python with the following packages installed:

```bash
pip install pygls lsprotocol
```

**Note:** The extension will attempt to use the Python interpreter configured in your VS Code `python.defaultInterpreterPath` setting. If not configured, it will use the system default Python.

#### Setting Python Path in VS Code

If the extension cannot find Python, configure it in your VS Code settings:

1. Open **Settings** (Ctrl+, or Cmd+, on Mac)
2. Search for `python.defaultInterpreterPath`
3. Set the path to your Python interpreter (e.g., `/usr/bin/python3` or `C:\Python312\python.exe`)

Alternatively, the extension will automatically detect Python from common locations.

## Building

```bash
# Development
npm run dev

# Build extension
npx vsce package
```

## License

MIT License - see [LICENSE](./LICENSE) file.

## Credits

Built with AI assistance from Gemini and MiniMax.
