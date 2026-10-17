# Current Analysis and Type Behavior

This is a reference for the 0.4.3 implementation (`e814e54`), including known
limitations. It is not an implementation plan or a complete Axon/Xeto language
specification. Future work and review decisions live in the
[planning index](plans/README.md).

## Sources and metadata

- Functions come from workspace sources/unsaved edits, configured external
  roots, source-backed `.xetolib` archives, and bundled catalogs. See the
  [README](../README.md#configuration) for settings and provider precedence.
- Legacy metadata comes from `coreFuncs.trio` and Haxall 3.1.12 sources. Modern
  bundled signatures come from Haxall 4.0.6 Xeto at commit
  `b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f`.
- Xeto requests prefer modern bundled signatures; other selection paths follow
  the configured metadata mode. Excluding bundled Xeto metadata retains legacy
  defs and indexed workspace/external/archive sources.
- Archives are read in memory. Virtual document URIs identify the archive path
  and entry, not a hash of changing file contents. Replacement/deletion and
  directory reconciliation refresh indexed sources; recovery remains bounded
  and depends on VS Code watcher delivery.
- The index stores function signatures and coarse Xeto spec names, not a
  qualified library/version namespace or structural slot graph. Older bundled
  catalogs may lack default metadata until regenerated.

## Expressions, bindings, and diagnostics

The Axon AST carries source spans with byte offsets and UTF-16 positions. It is
used alongside legacy line-oriented scanners; not every editor feature uses the
same interpretation or source mapping yet.

- Known literal categories include strings, URIs, booleans, numbers, references,
  dates, times, datetimes, date spans, `null`, lists, and dicts. Diagnostics use
  `None` internally for null; hover displays `null`.
- Embedded Xeto hover can use owner parameter types, local initializers, and
  indexed function-return signatures. Argument diagnostics use literal and
  available function-return types, but do not share a complete local-variable
  type evaluator with hover/completion.
- Both `value.func()` and `value.func` are checked as dot calls with an implicit
  first argument. Unresolved simple receivers/arguments, undefined functions,
  undefined assignment targets, missing required arguments, and primitive
  argument incompatibilities have diagnostic support. Unknown types are not
  intended to produce speculative mismatch warnings; exceptions caused by
  inconsistent resolution are listed below.
- Xeto types and `axon` defaults are stored separately. An explicit default
  permits omission without changing nullability. Defaults remain unevaluated
  Axon source, including metadata containing quoted commas/braces. Nullable
  parameters without explicit defaults retain the existing omission behavior;
  nullable returns are not treated as definite null/incompatible values.
- `->` is a source-spanned trap/tag lookup, supporting mixed trap/dot/index
  chains. Keys are literal names, not variables or function references. Trap
  result types remain unknown; there is no runtime record lookup or tag-existence
  check. Filter-typed arguments have special handling, with limitations below.
- `:` defines a binding; `=` requires an existing binding. The AST scope model
  handles same-line source order, function/lambda parameters, nested definitions,
  captures, and shadowing. Reassignment conservatively invalidates inferred
  value types rather than reusing stale initializer/parameter information.
  This is not branch- or invocation-sensitive flow analysis.
- Parser errors are retained internally but are not published as syntax
  diagnostics. Incomplete-arrow and multiline Trio `src:` recovery have focused
  coverage; the AST is not a complete compiler.

## Editor behavior

Signature help uses standard LSP, including nested/multiline AST call context
for embedded Xeto and implicit receiver positions. Trio also has AST call
context on the current line, with legacy word-based fallback. Some signature
parameter labels are still reconstructed from display strings.

Embedded Xeto hover recognizes literals, scoped parameters, selected local
initializers, and indexed function returns, including outer dotted chains.
Completion uses available type strings to prioritize function candidates and
add typed variable candidates without hiding unknown function candidates.
These are heuristic paths, not a single position-aware semantic service.

Definitions and function references primarily use the selected function index.
Lexical callable shadowing, local navigation, reference identity, and scope/type
queries are not uniformly shared across features.

## Known limitations

The post-0.4.3 investigation reproduced these behavior groups. Full examples and
code evidence are in the [foundation analysis](plans/analysis-foundation.md#2-evidence-and-baseline).

| Area | Current limitation |
| --- | --- |
| Resolution | Whitespace before call parentheses can change undefined-function reporting; local callables can incorrectly inherit global signatures |
| Scope/recovery | Unrelated parse errors can re-enable leaking heuristic parameter ranges; completion can offer out-of-scope locals; catch regexes can match strings |
| Mapping | Some consumers still mix byte columns with UTF-16 positions; embedded function references can retain body-relative rather than host line numbers |
| Lexical/grammar coverage | Multiline string contents can be scanned as code; defaulted/general trailing lambdas, partial application, dict shorthand, and try/catch are incompletely modeled |
| Filters/catch semantics | Filter exemptions can suppress checks on evaluated comparison values; catch binding lifetime and some compatibility forms differ from the pinned runtime |
| Signature presentation | A comma in a default expression can split one parameter into multiple displayed parameter entries |
| Type modeling | No flow-sensitive assignment inference, static trap-result types, structural conformance, or library dependency/version resolution |

## 0.4.3 validation record

The trap/reassignment changes passed `go test ./...`, `go test -race ./...`,
`go vet ./...`, `npm run test:client`, `npm run compile`, and `git diff --check`.
Manual VS Code smoke testing was reported passing by the user.

Regression coverage includes trap chains/key handling, call context and recovery,
Trio/Xeto diagnostic mapping, definition/reassignment ordering, and selected
closure/shadowing cases. Existing coverage also protects source/archive
precedence, replacement cleanup/reconciliation and diagnostic refresh, dot
receivers, Xeto default metadata, and temporal literal categories. Overlapping
receiver tests and repeated smoke setup were consolidated while retaining their
distinct assertions.

Passing checks validate the covered cases, not complete language support or
agreement between every analysis path. The known gaps above remain present.
