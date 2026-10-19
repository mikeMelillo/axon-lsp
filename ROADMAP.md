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
- Declared nominal ancestry for embedded Xeto hints and conservative argument checks,
  with a pinned qualified type catalog (structural fitting remains deferred)
- Function-value typing for named/inline callbacks, distinct from invocation returns
- Structured signature parameter/default labels and lexical call-target guards,
  including isolated multiline Trio source records

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

- 0.4.4
    - Retain `List` facts for literal-backed captured accumulators such as `results: []; recs.each(r => results = results.add(...))` when every write preserves List; share the fact across diagnostics and Xeto hover/completion without executing callbacks
    - Preserve declarations following an `if` without `else`; fix the full `firstPassPipeline` reproduction losing `results: []` after its preceding conditional, with empty/populated List and live-edit hover regressions
    - Preserve bindings initialized by chained lambdas containing `try do ... end catch ...`; try/catch no longer consumes the enclosing lambda's `end`
    - Keep leading-dot multiline method chains attached to their receivers, preventing `.map(...)` callbacks from being mistaken for the List argument
    - Show the `Func` type in known-function tooltips across host formats while retaining signatures and return types
    - Recognize named functions, inline lambdas, and initializer aliases as `Func` values without confusing them with invocation return types
    - Improve callback completion and hover while respecting lexical shadowing and conservative reassignment invalidation
    - Build signature parameters from structured type/default metadata, with a quote/nesting-aware legacy fallback so commas inside defaults do not create phantom parameters
    - Prevent local callbacks, parameters, and aliases from borrowing unrelated global signatures; retain unknown local signatures rather than guessing
    - Use containing Trio source records for multiline signature help, preserving inline `src:` columns and isolating bindings between records
    - Add focused regressions and update READMEs, plans, and documented callback/type-inference limitations
    - Automated tests, race checks, vet, client checks, cache verification, and all six server builds passed; manual VS Code smoke testing pending
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
