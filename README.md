# Axon Language Server (axon-lsp)

> **⚠️ POC Notice**: This extension was built with AI assistance (Gemini + MiniMax) as a proof-of-concept. Use at your own risk. 


A VS Code extension that provides Language Server Protocol (LSP) support for the Axon programming language used in SkySpark and Haxall.

This is my first whirl at publishing a tool like this so -- you've been warned! I'm hoping to get this working reasonably well circa 3.1.12 and then continue to update once we're in 4.0 to make development easier. I think there are some fun opportunities to add type hints, linting, debuggin into VS Code which should make day to day engineering much easier.

## Features

- **Autocomplete**: Intelligent code completion for Axon functions
- **Hover Information**: View function documentation on hover
- **Go to Definition**: Navigate to function definitions within workspace (links out to core lib & haxall WIP)
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
[Market Place Install](https://marketplace.visualstudio.com/items?itemName=mikeMelillo.axon-lsp)

## Configuration & Dependencies

The server is written in Python on top of pygls (lsprotocol and cattrs are bundled with pygls). Any Python rev newer than 3.11 has ran fine in testing. Pygls should be version 1.3.X. If the server remains in Python I will eventually look at migrating to pygls 2. 

`pip install pygls==1.3.0`

I understand this adds some friction to using the extension, but it allowed for the quickest path to do what really amounts to an experimental / open source project first for my own use. **In the future, I would consider a server rewrite in Go, so if you have opinions there, please reach out.**

Beyond that, there's currently no configuration required. This extension has cached the core haxall & skyspark function library, and syncs automatically with your local working directory. This is nice, because you don't need to have the Haxall repo cloned onto your machine to make use of the extension.

Future versions will support further customization or multiple directory support.

## How It Works

Axon LSP has sourced functions from:

- Your local working directory
- The open source Haxall Repo
- Known SkySpark functions (function name & doc string only, no argument hints currently)

The following file types and function types are parsed:

- Support version 3.1.12 only (4.0 future)
- Supports parsing `func` recs from `.trio` or `@Axon` from `.fan`
- Supports parsing `defcomp` functions
- No other file types or recs are read in for LSP caching

## Roadmap

See [ROADMAP.md](./ROADMAP.md) for features under development.

### Note

In the interest of keeping this extension small and (hopefully) fast, I've cached core functions into a small file rather than include the entire haxall repo, or making REST calls out to github to update. For that reason, reference linking to the Haxall core lib functions is not currently supported, exploring the best way to do this in the future (either hyperlink out to github web page at that .fan file or allow user to set directory to a local haxall clone for in-editor loading).

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
