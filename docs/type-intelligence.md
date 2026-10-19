# Current Analysis and Type Behavior

This is a reference for the current 0.4.5 implementation, including cache/T0 nominal
inheritance and known limitations. It is not an implementation plan
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

Trio document diagnostics extract and validate only `src:` regions, masking the
field label without shifting inline columns. Other record fields are data:
`ruleOn: room and networkRef->dis=="EPMS"` is not an Axon receiver expression.
Each region is analyzed independently, with diagnostic and reference line numbers
mapped back to the host document. References are cleared once per document and
then accumulated from its source regions; deleting all source regions clears old
references. There is no raw-Axon fallback for a Trio document without `src:`.
Explicit region-validation APIs still accept already-extracted Axon fragments.
This is a diagnostics boundary fix, not a migration of every editor feature or
TextMate grammar to region-aware analysis.

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
- Leading-dot continuation lines stay attached to the previous receiver, so
  multiline `.map(...)` lambdas bind to the callback slot, not the List slot.
  `try do ... end catch <expr>` and block catchers own only their own blocks;
  they do not consume an enclosing lambda's `end` or following method chain.
  Initializer bindings remain visible after those chains. The older bare
  multiline `try ... catch ... end` editor form remains a compatibility path;
  this is not full grammar/recovery or a change to catch-binding lifetime.
- Optional-`else` lookahead preserves newlines when no `else` follows. Inline,
  block, and nested conditionals therefore do not cause recovery to discard the
  next declaration's name. Newlines before an actual `else` still join its branch.
- Function values have type `sys::Func`: an unshadowed indexed function name,
  a lambda, and local initializer aliases of either are callable values. This
  does not use or evaluate the function body. For example, `jevCallback` is a
  `Func`, while `jevCallback(code, headers, body)` has its declared `Dict?`
  return type. Diagnostics classify these values rather than merely skipping
  lambdas or treating defined function names as untyped. Alias lookup is bounded
  and respects lexical shadowing and conservative reassignment invalidation.
  This does not check callback parameter counts or return-signature conformance.
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
  captures, and shadowing. Reassignment generally invalidates inferred value
  types rather than reusing stale initializer/parameter information. The bounded
  List-invariant exception is described below.
  This is not branch- or invocation-sensitive flow analysis.
- Parser errors are retained internally but are not published as syntax
  diagnostics. Incomplete-arrow and multiline Trio `src:` recovery have focused
  coverage; the AST is not a complete compiler.
- Parenthesized lambdas recognize required/defaulted parameter mixes, including
  nested default expressions and multiline parameter lists. Parameter names enter
  the lambda's lexical scope regardless of whether a default is present. Trio
  signature extraction shares this grammar and retains the original rendered
  signature rather than splitting nested defaults at every comma. Defaults are
  parsed to recognize the header but not evaluated, walked as body declarations,
  or used to infer parameter types/check local callable arity. Default-expression
  name validation and invocation semantics remain outside this bounded fix.

### Captured List accumulators

`results: []; recs.each(r => results = results.add(r.processRec()))` parses as a
closure assigning the existing outer binding. Its List fact no longer disappears
at that assignment: `scope.Model.IsListValue` proves a literal-backed List seed
and List-producing RHSs for **every** write to that exact lexical binding,
including deferred closures and later-source writes. Nested shadowing is a
different binding. Unknown/type-changing writes defeat the invariant; the query
does not assume that a closure ran, or how often it might run.

Supported facts are List literals/groups, bounded literal-backed aliases, and
fully applied built-in `add(list, item)` / `list.add(item)` with a proven List
receiver. This is a small on-success List-category rule, not element/shape typing
or a guarantee that evaluating an argument cannot throw. It is gated to the
selected pinned bundled built-in, never a lexical parameter/local or an indexed
workspace/external override named `add`. Legacy bundled defs retain that built-in
rule when bundled Xeto metadata is excluded. Other providers, declared parameter
contracts, arbitrary return signatures, partial application, unsupported writes,
and exhausted 128-step proofs do not establish this invariant.

The same proof feeds Trio/Xeto argument diagnostics and embedded Xeto variable
hover, argument completion, and receiver ranking, including reads within the
assignment RHS. Tests assert a concrete List fact (including a Number-contract
mismatch), not just absence of a diagnostic. Normal declared-return lookup is
unchanged: `each(...)` itself is not inferred to return the accumulator List.
General reassignment/branch/invocation inference remains T1 work.

### Explicit native defaults (0.4.5)

The Trio infeed follow-up also recognizes dictionary marker/removal entries
(`{flag}`, `{-limit}`) in default expressions. Previously the unsupported map
entry made lambda-header recognition fail, losing required parameter bindings.
The full infeed pipeline regression now preserves `infeed`; a deliberately
undefined receiver still warns. Go tests/race/vet and six server builds passed
after this parser fix. Manual editor retesting remains outstanding.

The shared Xeto parser separates explicit scalar defaults from type names:
`checked: Bool "true"` and `checked: Bool <val:"true">` preserve Bool typing and
permit omission. This also works in indexed sources and archives. Only explicit
slot values are considered, never inherited type-instantiation values. Existing
`<axon:"...">` expressions remain supported and take precedence when both forms
are present. Known Bool, Str, and simple numeric defaults are normalized for
display; other encodings retain their typed Xeto spelling. Default strings are
display/omission facts, not inputs to an interpreter or parameter-type inference.

The [native supplement](cache-baseline.md#native-default-supplement) supplies 11
reviewed defaults for 10 older bundled functions, using exact-revision source
evidence. It preserves explicit upstream defaults and does not change provider
selection or enrich unrelated workspace/external functions. Unreviewed revisions
and dynamic defaults remain outside coverage. `read(site)` and `specs()` can omit
their recovered defaults; `read()` still needs `filter`, and defaulted Bool
inputs do not gain explicit null acceptance. Provenance is retained in the
generated cache artifact; no upstream source bytes are changed.

Verification: Go tests and the full race suite, vet, client tests, TypeScript
compilation, deterministic cache verification, all six server builds, and
whitespace checks passed. Source/archive/cache transfer and override-isolation
regressions pass. A rebuilt-server stdio probe accepted `read(site)`, `specs()`,
and `"42".parseNumber`, preserved the missing-filter and explicit-null warnings,
and displayed `read(filter: Filter, checked: Bool = true)` in signature help.
Manual VS Code smoke testing of this follow-up remains outstanding.

### Language semantics reviewed for this patch

The upstream Haxall 4.0.6 chapters reviewed before the accumulator change were
[Architecture](https://haxall.io/doc/hx.doc.haxall/Architecture),
[Runtime](https://haxall.io/doc/hx.doc.haxall/Runtime),
[Namespace](https://haxall.io/doc/hx.doc.haxall/Namespace),
[Axon](https://haxall.io/doc/hx.doc.haxall/Axon),
[Axon Language](https://haxall.io/doc/hx.doc.haxall/AxonLang), and
[Axon Usage](https://haxall.io/doc/hx.doc.haxall/AxonUsage). In particular:

- The runtime's enabled libraries/companion functions define its namespace;
  editor provider precedence is not a live runtime namespace solver.
- Axon is dynamically typed and functional. Dot calls pass an implicit first
  argument; omitting parentheses from a bare function name produces the Func.
- `:` defines, `=` reassigns, and closures capture lexical outer bindings;
  ordinary `do`/`if`/`try` blocks do not introduce function scopes.
- Lists are immutable; built-in `add` returns a new List and does not mutate the
  old one. The pinned `AxonFuncs.fan` implementation and `funcs.xeto` declarations
  agree; the declaration's broad `Obj?` return is not changed in the cache.

These sources guide the focused fix, not a claim of complete language support.
Known parser, catch, filter, and editor-resolution differences remain listed
below; no runtime/database access is added to analysis.

## Editor behavior

Signature help uses standard LSP, including nested/multiline AST call context
for embedded Xeto, containing Trio `src` records, and implicit receiver positions.
Trio records are isolated and the `src:` field label is masked without shifting
inline columns. Structured parameter names/types/default source build the labels;
older signatures use a lexical string/nesting-aware fallback. Dictionaries,
lists, nested calls, strings, and lambda defaults containing commas remain one
parameter. Parsed-owner fallback uses the same label builder as indexed functions.

The callee is resolved at its exact UTF-16 source occurrence before global
metadata is selected. A local lambda, owner/lambda parameter, alias, or reassigned
binding yields no signature help rather than borrowing a same-named global's
contract. Local signature inference is not implemented. The lookup does not use
a shadow introduced only inside a later argument, and no unrelated earlier word
can supply a signature after an unknown/shadowed real call target is found.
Bare-name fallback is limited to the name at the cursor, excluding strings and
trap keys. Other features' local definition/reference resolution remains separate
F2 work; this is not a complete editor-wide name-resolution migration.

Embedded Xeto hover recognizes literals, scoped parameters, selected local
initializers, and indexed function returns, including outer dotted chains.
Completion in embedded Xeto uses the shared nominal relation to rank receivers
against first parameters, and function returns/scoped variables against active
argument types. It keeps unknown function candidates visible. Variable candidates
use exact cursor visibility and existing conservative mutation invalidation.
These are bounded inference paths, not a complete shared flow-sensitive service.

For a `Func`-typed argument (including `sys::Func?`), completion offers named
callbacks as function values regardless of their return types, alongside typed
local lambdas/aliases. Other argument slots retain return-type ranking for
potential function invocations. A shadowing local replaces the same-named global
function suggestion instead of inheriting its callable identity. Local callback
hover displays `Func`; the shared indexed-function hover builder identifies the
`Func` value across host formats while retaining its full signature and declared
return type. Embedded Xeto no longer adds a separate duplicate annotation.
Bare names in Filter-typed slots remain tag syntax, even when an indexed function
shares the name.

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
| Resolution | Whitespace before call parentheses can change undefined-function reporting; definition/reference and some hover paths are not yet uniformly scope-aware (signature help now guards local callees) |
| Scope/recovery | Unrelated parse errors can re-enable leaking heuristic parameter ranges; catch regexes can match strings; not all features use exact-position binding queries |
| Mapping | Some consumers still mix byte columns with UTF-16 positions; embedded function references can retain body-relative rather than host line numbers |
| Lexical/grammar coverage | Multiline string contents can be scanned as code; general trailing lambdas, partial application, dict shorthand, and try/catch are incompletely modeled; defaulted parameter headers are recognized, not fully semantically checked |
| Filters/catch semantics | Filter exemptions can suppress checks on evaluated comparison values; catch binding lifetime and some compatibility forms differ from the pinned runtime |
| Local callable signatures | Function values are recognized, but local lambda/alias invocation signatures and result types generally remain unknown; signature help deliberately does not substitute globals |
| Type modeling | No flow-sensitive assignment inference, static trap-result types, structural conformance, or library dependency/version resolution |

Callback parameter count/type/return conformance is not checked. Generic
higher-order returns are not specialized: `ioHttp(..., jevCallback)` is still
`Obj?` even when `jevCallback` declares `Dict?`. Function-body return expressions
are not validated against declared return types (for example, `return res` in a
function declaring `Str?`). No callback execution, HTTP request, or runtime
inspection is part of inference.

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

## 0.4.4 callback/signature verification (2026-10-17)

The callback-value fixes and both signature-help follow-ups passed `go test ./...`,
the full race suite, `go vet`, client tests, TypeScript compilation, read-only
cache verification, all six server builds, and whitespace checks. Regression
coverage includes complex defaults, parsed-owner fallback, local/global callee
shadowing, active parameters, UTF-16 positions, and isolated Trio records.
Manual VS Code smoke testing remains outstanding for this working build; the
README lists suggested checks. No cache regeneration or version bump was needed.

### Parser/tooltip follow-up (2026-10-18)

The reported `allTags` try/catch initializer and both multiline regex/map chains
are covered by AST and Trio/Xeto diagnostic regressions, including lambda-local
scope boundaries and `Func` tooltips. Full Go tests/race/vet, client checks,
TypeScript compilation, cache verification, six server builds, and whitespace
checks passed again. This remains part of 0.4.4; manual smoke testing is pending.

### Captured List accumulator follow-up (2026-10-18)

After reviewing the upstream architecture/Axon chapters, the `results: []`
closure-assignment example passed parser-ownership, lexical invariant, diagnostic,
and Xeto hover/completion regressions. Coverage includes repeated/block callback
writes, shadowing, unknown/type-changing writes, selected-function overrides,
legacy metadata selection, partial-application rejection, and proof budgets.
Full Go tests/race/vet, client tests, TypeScript compilation, read-only cache
verification, all six server builds, and whitespace checks passed. No cache/schema
change or version bump was needed; manual VS Code smoke testing remains pending.

### Empty versus populated List recheck (2026-10-18)

Follow-up feedback reported `results: []` not being recognized while
`results: [123]` was. Both are valid List literals. The smaller fragment passed
literal, hover, live-edit, and built-server probes, but the subsequently supplied
full `firstPassPipeline` function reproduced the failure: optional-`else`
lookahead consumed the newline after
`if(inputSpecs.isNull) inputSpecs = specs()`. Recovery then discarded the next
declaration's name and reported undefined assignment/receiver uses of `results`.
The issue was statement parsing, not empty-list typing or `List<of:...>` metadata.

The parser now restores those newlines unless an `else` actually follows. AST
regressions cover inline/block/nested conditions, real else branches, blank-line
and semicolon separators, and both List forms. The full user function replaces
the smaller live-edit hover fixture: `[]` → `[123]` → `[]` must retain the
declaration, bracket literal, callback RHS, and returned List without undefined
`results` diagnostics. This remains a focused 0.4.4 parser fix, not branch-sensitive
type inference. Manual VS Code retesting remains outstanding.

Full Go tests/race/vet, client tests, TypeScript compilation, read-only cache
verification, all six server builds, and whitespace checks passed after the
statement-boundary fix. The rebuilt Linux server also passed all 12 stdio hover
checks on the full function across the three List edits, with no undefined
`results` diagnostics.

A separate warning originally remained with only bundled metadata: the pinned
`hx.xeto::specs` Xeto signature declares `filter: Filter` without an Axon default,
although the Fantom implementation defaults both arguments to null. The live-edit
regression supplies controlled helper signatures to isolate the List/parser case;
the built-server probe confirmed this unrelated missing-filter warning. No cache
or upstream signature was rewritten as part of this fix.

The subsequent 0.4.5 default-metadata follow-up below resolves that bundled
`specs()` omission warning through a source-verified supplement.

### Trio mixed-parameter follow-up (2026-10-19)

The exact `sensorLocationToAisle` example now has no parameter-related warnings.
Regressions cover required/all-defaulted/mixed headers, nested and multiline
defaults, preserved Trio display signatures, lexical boundaries, and a genuine
undefined receiver with its host line. The rebuilt Linux server also passed
open/change probes: original source clean, `missingLocation` typo detected,
restored source clean. Go tests/race/vet, client tests, TypeScript compilation,
read-only cache verification, six server builds, and whitespace checks passed.
No version bump or cache regeneration was needed; manual editor retesting remains
outstanding.

### Trio source-region follow-up (2026-10-19)

The complete `aeFdcim_mechLoadVsDegreeDay` record passes without diagnostics;
`networkRef` in `ruleOn` is no longer checked as an Axon variable. An undefined
receiver injected inside the defcomp source still warns at its original document
position. Tests also cover record-scope isolation, metadata names not declaring
body variables, inline source columns, source-only references, and reference
cleanup when source is removed. The rebuilt Linux server passed open/change
probes for the original rule, injected body error, and a metadata-only record.
Full Go tests/race/vet, client tests, TypeScript compilation, cache verification,
six server builds, and whitespace checks passed; manual editor retesting remains
outstanding.
