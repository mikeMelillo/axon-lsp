# Current Analysis and Type Behavior

This is a reference for the 0.4.3 implementation plus the cache/T0 nominal
inheritance work, including known limitations. It is not an implementation plan
or a complete Axon/Xeto language specification. Future work and review decisions live in the
[planning index](plans/README.md).

## Sources and metadata

- Functions come from workspace sources/unsaved edits, configured external
  roots, source-backed `.xetolib` archives, and bundled catalogs. See the
  [README](../README.md#configuration) for settings and provider precedence.
- Legacy metadata comes from `coreFuncs.trio` and Haxall 3.1.12 sources. Modern
  bundled signatures come from Haxall 4.0.6 Xeto at commit
  `b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f`.
- The [cache baseline](cache-baseline.md) now also includes the separate Project
  Haystack Xeto 5.0.0 standard-library sources, build properties, and provenance.
  Its additive `type_catalog.json` supplies the declared nominal base graph;
  legacy Haxall 3.x metadata is not mixed into that graph.
- Xeto requests prefer modern bundled signatures; other selection paths follow
  the configured metadata mode. Excluding bundled Xeto metadata retains legacy
  defs and indexed workspace/external/archive sources.
- Archives are read in memory. Virtual document URIs identify the archive path
  and entry, not a hash of changing file contents. Replacement/deletion and
  directory reconciliation refresh indexed sources; recovery remains bounded
  and depends on VS Code watcher delivery.
- The index stores function signatures, coarse spec names, and a separate
  qualified nominal catalog. It is not a strict versioned namespace or structural
  slot graph. Older bundled catalogs may lack default metadata until regenerated.

## Expressions, bindings, and diagnostics

The Axon AST carries source spans with byte offsets and UTF-16 positions. It is
used alongside legacy line-oriented scanners; not every editor feature uses the
same interpretation or source mapping yet.

- Known literal categories include strings, URIs, booleans, numbers, references,
  dates, times, datetimes, date spans, `null`, lists, and dicts. Diagnostics use
  `None` internally for null; hover displays `null`.
- Embedded Xeto hover can use owner parameter types, local initializers, and
  indexed function-return signatures. Argument diagnostics use literal,
  function-return, and declared owner-input types, but do not share a complete
  local-variable type evaluator with hover/completion.
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
Completion in embedded Xeto uses the shared nominal relation to rank receivers
against first parameters, and function returns/scoped variables against active
argument types. It keeps unknown function candidates visible. Variable candidates
use exact cursor visibility and existing conservative mutation invalidation.
These are bounded inference paths, not a complete shared flow-sensitive service.

Definitions and function references primarily use the selected function index.
Lexical callable shadowing, local navigation, reference identity, and scope/type
queries are not uniformly shared across features.

## Nominal compatibility (T0)

`internal/typeinfo` resolves simple names in their declaring source context and
compares qualified, provider-specific identities. It follows declared base chains,
including `Int → Number → Scalar → Obj` and
`Entity → Dict → Collection → Obj`. Parameter and return type spellings do not
lose their source context; unrelated libraries' same-spelled types are distinct.
Axon's date-span literal category maps to the catalog's `sys::Span` identity.

Library selection uses coherent providers, not per-type fallback: workspace
editable trees, workspace archives, configured external roots in order (editable
before archive within a root), then the modern bundle. A library is identified
by its source directory with `lib.xeto`; loose files share an anonymous directory
scope. Root-level archives without a named library directory also remain
anonymous; filenames/URI hashes are not library identities. Imports include
`sys` and literal manifest dependency names. This is editor
precedence, **not dependency/version solving**. A lower-priority signature cannot
resurrect a superseded library's graph.

The directional result is compatible, definitely incompatible, or unknown.
Missing/ambiguous metadata, duplicate declarations, cycles, unsupported compound
headers, and incomplete/over-budget ancestry remain unknown. Failure to inherit
does not prove failure to fit structurally; custom specs do not gain blanket
mismatch warnings. Definite intrinsic category mismatches and null supplied to a
resolved non-null contract still warn. Nullable values/default omission retain
their previous rules, including the `Obj` special
case; numeric subtype/unit/range constraints are not inferred from literals.

Catalog snapshots refresh with source/manifest edits, removal, archive rebuilds,
root changes, and settings. `ignoreBundledXetoCache` excludes bundled ancestry as
well as bundled function variants, without suppressing indexed sources or
intrinsic primitive checks. Traversal is capped at 128 nodes; initializer
inference at 32 recursive steps. No runtime, downloads, or Haxall execution occurs.

## Known limitations

The post-0.4.3 investigation reproduced these behavior groups. Full examples and
code evidence are in the [foundation analysis](plans/analysis-foundation.md#2-evidence-and-baseline).

| Area | Current limitation |
| --- | --- |
| Resolution | Whitespace before call parentheses can change undefined-function reporting; local callables can incorrectly inherit global signatures |
| Scope/recovery | Unrelated parse errors can re-enable leaking heuristic parameter ranges; catch regexes can match strings; not all features use exact-position binding queries |
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

## T0 validation record (2026-10-17)

The nominal milestone passed Go tests (including the full race suite), `go vet`,
client tests, TypeScript compilation, cache regeneration/read-only verification,
all six server cross-builds, and whitespace checks. Focused coverage includes
graph direction, ambiguity/missing/cyclic/incomplete metadata, traversal limits,
qualified return provenance, typed inputs, cursor-aware completion ranking,
provider replacement/removal, and live bundled-cache exclusion. No new manual
VS Code smoke test is claimed.
