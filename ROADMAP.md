# Axon LSP Roadmap

## Version 0.4.x - Current Direction

### Completed

- Go-based language server bundled directly in the VSIX
- No external Python runtime requirement
- Basic autocomplete for Axon functions
- Hover information
- Go to definition with local/core/external support
- Diagnostics for undefined functions
- Support for `defcomp` syntax
- Basic syntax highlighting
- Function signature help
- Reference finder
- Document symbols, multi-root indexing, and configurable external sources
- Xetolib archive indexing and read-only source navigation
- Current-file and editable-source precedence over archived metadata
- Optional bundled Xeto metadata, with legacy defs retained
- Dot-call receiver validation and explicit Xeto parameter-default metadata
- Source-spanned trap (`->`) expressions, tag-path handling, and mixed call/index chains
- Position-aware definition/reassignment diagnostics and conservative mutation typing

### Phase B - Legacy Support Polish

- Expand cached core and SkySpark function coverage
- Improve hover and definition ergonomics
- Improve workspace indexing and multi-directory support
- Continue tightening parser and diagnostic edge cases
- Expand syntax recovery/reporting without speculative diagnostics on partial code

### Phase C - Richer Xeto Intelligence

- Prepare for the migration away from `defcomp`
- Introduce spec-oriented symbol and type modeling
- Extend existing Xeto workflows with richer type metadata and structural conformance
- Add library dependency/version selection beyond source-root archive precedence
- Add flow-sensitive variable inference; trap results currently remain unknown
- See `docs/type-intelligence.md` for the active type-intelligence plan.

## Architecture Notes

### Current (Legacy and Haxall 4.x Sources)

- Indexes `.fan` files with `@Axon` annotations
- Indexes `.trio` files with `func` records
- Indexes Xeto functions/spec names and in-memory `.xetolib` source documents
- Parses embedded Axon expressions and lexical bindings for conservative diagnostics
- Provides a cached core-function backstop
- Uses a VS Code TypeScript client with a bundled Go server over stdio

### Further Work

- Expand the existing Xeto name/function index into richer spec and library dependency modeling
- Type-aware editor features will likely move beyond function-only metadata

## Recent Notes

- 0.4.3
    - Add trap-operator AST support and tag-safe hover, definition, and reference lookup
    - Separate variable definitions from reassignment with position-aware lexical binding resolution
    - Preserve signature help around trap arguments and recover following statements after incomplete input
    - Consolidate overlapping diagnostic tests and refresh documentation
    - Automated checks and user-reported manual VS Code smoke testing passed
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
