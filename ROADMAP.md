# Axon LSP Roadmap

## Version 0.4.x - Current Direction

This is the product-level roadmap. Detailed implementation proposals and their
review status live in the [planning index](docs/plans/README.md); implemented
behavior and known limitations are documented in the
[analysis and type reference](docs/type-intelligence.md).

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

### Direction

- Make parsing, recovery, source mapping, and name resolution more consistent
  across editor features before adding broader diagnostics.
- Add useful type inference incrementally, preserving unknown results where
  runtime behavior or metadata is incomplete.
- Extend Xeto support with qualified spec/slot modeling, offline library/version
  contexts, and carefully bounded structural conformance.
- Continue legacy workflows and catalog coverage while supporting the 4.x shift
  away from `defcomp`.
- Explore snippets, optional code-quality checks, and preservation-safe formatting
  after the analysis foundation is dependable.

The planning index owns priorities and the deferred backlog; its detailed plans
own delivery sequences and acceptance criteria. Proposals are not release
commitments.

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
