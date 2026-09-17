# Axon LSP Roadmap

## Version 0.2.x - Current Direction

### Completed

- Go-based language server bundled directly in the VSIX
- No external Python runtime requirement, it was nice for a POC but will end up a long term PITA
- Basic autocomplete for Axon functions
- Hover information
- Go to definition with local/core/external support
- Diagnostics for undefined functions
- Support for `defcomp` syntax
- Basic syntax highlighting
- Function signature help
- Reference finder

### Phase B - Legacy Support Polish

- Add `Ctrl+Shift+O` / document symbols
- Expand cached core and SkySpark function coverage
- Improve hover and definition ergonomics
- Improve workspace indexing and multi-directory support
- Continue tightening parser and diagnostic edge cases

### Phase C - 4.x / Xeto Preparation

- Prepare for the migration away from `defcomp`
- Introduce spec-oriented symbol and type modeling
- Support Xeto-backed workflows and richer type metadata
- Adapt indexing around the upcoming 4.x architecture

## Architecture Notes

### Current (Haxall 3.1.12)

- Indexes `.fan` files with `@Axon` annotations
- Indexes `.trio` files with `func` records
- Provides a cached core-function backstop
- Uses a VS Code TypeScript client with a bundled Go server over stdio

### Future (Haxall 4.x)

- Architecture changes are expected to be substantial
- Specs/Xeto will need their own indexing and symbol model
- Type-aware editor features will likely move beyond function-only metadata

## Recent Notes

- 0.2.x
    - Replace the Python server with a bundled Go server
    - Cross-compile extension binaries for the 6 supported desktop targets
    - Replace cache generation with a Go builder
- 0.1.8
    - Inline object detection for function-definition false positives
    - Function signature support added
    - Exclude detection within `doc` and `summary` tags
    - Reference finder support added
- 0.1.5 - 0.1.7
    - Add `//lspignore` for single-line omissions
    - Correct local Fantom function indexing
    - Omit `Text(args)` patterns inside strings correctly
    - Add detection for local helper functions
