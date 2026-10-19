# Type-Model Evolution

Status: **T0 implemented in the current working changes; T1–T3 remain drafts for
review, not implemented or approved release scopes**.

Analysis date: 2026-10-16. Baseline: `e814e54` (0.4.3).
Foundation plan for the broader T1–T3 work: [analysis foundation](analysis-foundation.md).
Current behavior: [analysis and type reference](../type-intelligence.md).

## 1. An early milestone and three later change paths

- **T0 — Early declared nominal inheritance:** a lightweight qualified base
  catalog and shared directional compatibility queries. Delivered independently
  of structural fitting and the broader foundation projects.
- **T1 — Flow-sensitive reassignment inference:** recover useful types at an
  exact program position without inventing closure/branch execution.
- **T2 — Static trap-result typing:** use trustworthy literal shapes or resolved
  Xeto slot metadata; never read live records or infer from a key's spelling.
- **T3 — Structural conformance and library/version modeling:** two separate
  projects sharing a qualified metadata foundation, not a single large checker.

The foundation plan's F1/F2/F3 are different numbered items. In particular,
syntax diagnostics are not a prerequisite UI feature for T1; reusable parsing,
mapping, recovery confidence, and semantic name identities are prerequisites.

## 2. What the current implementation actually provides

| Area | Baseline | Consequence for the next changes |
| --- | --- | --- |
| Binding model | `scope.Analyze` records initializer text, declaration span data, and assignments as name/position pairs | Cannot evaluate the RHS of a write or distinguish executed writes from deferred closure writes |
| Mutation handling | `scope.ResolveAt` clears Type/Expression after an earlier source-positioned write resolves to the same binding | Safe loss of precision, not flow analysis; no branch join or call effect model |
| Editor queries | Completion enumerates visible bindings; signature help guards the callee's exact position; some hover/type helpers still use line-end `Resolve` | Finish shared occurrence/position resolution before stronger type claims |
| Expression types | Diagnostics use literals/global returns/typed owner inputs; callable-value recognition is shared with LSP; other initializer inference is still separate | Nominal compatibility and Func facts are shared, not full expression inference |
| Type representation | Signature fields remain strings; T0 attaches their declaration URI when querying qualified nominal identities | No generic/ref target, union/intersection, or complete shared value-fact model |
| Null | Semantic literal type is `None`, hover renders `null`, nullable returns suppress definite mismatch claims | Preserve these distinctions in a shared internal model, not separate string conventions |
| Xeto specs | Coarse `HasSpecName` remains; T0 separately parses simple top-level bases and contextual identities | No effective slots, mixins, structural fitting, or strict versioned namespace |
| Function catalog | Provider precedence works, but symbols are primarily keyed by simple function names; `Qualified` is not populated by the current adapters | Source precedence must not be mistaken for namespace or dependency resolution |
| Xeto metadata | Signature parser retains type strings and `axon` defaults, with special handling for `maybe` | Important metadata such as `of` is not a semantic TypeRef; adding trap types needs richer data ingestion |
| Archives | Sources are read in memory, with bounded ZIP parsing and atomic replacement/purge | Reuse lifecycle/security behavior; archive URI hashes identify the provider path, not its version or contents |
| Bundled assets | Function/source assets plus versioned `type_catalog.json` with modern library provenance and declared bases | Extend the nominal subset for effective slots; do not claim the catalog is a Xeto compiler |

Relevant Go entry points:

- `internal/scope/{scope,function}.go`: `Analyze`, `ResolveAt`, `Resolve`,
  `IsFunctionValue`.
- `internal/diag/validator.go`: `argumentExprType`, `optionalType`, and
  `validateCallsFromAST`; `internal/typeinfo/nominal.go`: shared `Relate`.
- `internal/lsp/server.go`: `embeddedExpressionType`, `embeddedLocalType`,
  `expectedCompletionType`, `scopeVariables`, and embedded hover/signature paths.
- `internal/xeto/parser.go`: `ParseURIContent`, `ParseSpecNames`,
  `parseFuncSignature`, and metadata splitting.
- `internal/index/{types,manager,archive}.go`: identity, selection, spec names,
  provider replacement, and reference storage.
- `internal/index/signature.go` and `internal/lsp/signature.go`: structured
  signature labels, bounded legacy fallback, and lexical call-target guards.
- `internal/cache/cache.go` and `cmd/cache-builder/main.go`: serialized metadata.

## 3. Common type and identity contract

Before expanding diagnostics, separate **what is known** from a displayed type
name. A minimal internal fact should carry:

- Type identity or type expression, distinct from a source spelling.
- Observed/inferred versus declared/expected type information.
- Definite null, maybe-null, and unknown as different states.
- Evidence/provenance (literal, parameter contract, resolved return, write,
  merged branches, recovery, ambiguous metadata).
- Optional closed shape information and per-slot presence/type facts.
- Source/binding/spec identities and the revision/generation they depend on.

Unknown is not `Obj`, missing, null, or incompatible. `Obj` is a real type
contract. Type names alone do not establish whether a shape is closed or whether
an assignment executed. Resolution should distinguish resolved, absent in a
complete context, ambiguous, and unavailable/incomplete metadata.

A shared expression/type-query layer should consume AST/binding snapshots and a
read-only catalog adapter. Both diagnostics and editor ranking/hover must call
it. A neutral `internal/types` or analysis subpackage is a possible boundary;
keep LSP JSON types and index implementation out of the core to avoid cycles.
Do not build a general-purpose type framework before the first useful queries.

T0 normalizes known builtin spellings conservatively and retains unresolved type
text. `Number` is not equivalent to an unrelated library's same-spelled type.
Its minimal contextual identities are the starting point for richer T3 metadata,
not a replacement for a complete value-fact or versioned namespace model.

## 3a. T0: early declared nominal inheritance

**Implemented ahead of T1–T3.** Full parser recovery, flow-sensitive inference,
structural checking, and dependency/version solving are not prerequisites.

- `internal/typeinfo` owns a directional three-way relation and immutable
  catalog. `Int → Number → Scalar → Obj` and
  `Entity → Dict → Collection → Obj` come from the pinned standard sources.
- The cache builder emits an additive schema-versioned `type_catalog.json`:
  library/type identities, simple declared bases, document completeness, source
  URIs, provider IDs, versions, and pinned revisions. It excludes the legacy
  Haxall 3.x graph instead of mixing revisions by spelling.
- Workspace/external sources and archive entries use the same narrow parser.
  Named library directories require a `lib.xeto`; loose source directories are
  anonymous contextual scopes. Imports are literal manifest dependency names
  plus `sys`. Version constraints/BuildVars are not solved by this query.
- Whole-library precedence is workspace editable, workspace archive, configured
  external root order (editable before archive within a root), then bundled.
  Equally ranked copies, duplicate declarations, missing/ambiguous imports,
  unsupported headers, missing bases, cycles, and over-budget paths yield unknown.
  A selected library's missing declaration is never filled from a lower copy.
- A type spelling stays attached to its declaring source, including selected
  function returns. Identity is qualified and provider-specific, not a simple
  name comparison. Superseded providers cannot reappear through signature URIs.
- Receiver completion compares actual receiver to candidate first parameter.
  Argument completion compares variables/function returns to the active expected
  parameter. Both use the shared relation; unknown function candidates remain
  visible. Scoped suggestions respect cursor visibility and mutation invalidation.
- Argument diagnostics propagate declared owner input types and reuse the same
  relation. Positive ancestry is accepted, but a failed nominal search is **not**
  a structural mismatch. Definite intrinsic category failures and null supplied
  to a resolved non-null contract warn.
- Existing omission defaults, nullable-return conservatism, and the special
  `Obj`/null policy are retained. Unknown is not `Obj`, null, or an undefined name.
  Numeric subtype constraints are not enforced from literal spelling.
- Index replacement publishes a fresh immutable catalog on Xeto/manifest edits,
  removal, archive replacement/purge, root changes, and bundled-cache settings.
  Disabling bundled Xeto hides its graph; intrinsic primitive checks and indexed
  sources remain. Traversal is capped at 128 nodes and initializer queries at 32
  recursive steps; no graph memoization or runtime execution is introduced.

Focused tests own lexical extraction, graph direction/ambiguity/budgets,
generated provenance, provider lifecycle, completion direction, and typed-input
diagnostics. This is nominal information only: required fields, effective slots,
compound/mixin types, ref targets, numeric fidelity, dependency constraints,
flow-sensitive writes, and trap-result typing remain deferred. Completion type
ranking currently applies to embedded Xeto bodies; this is not a broad Trio
completion/source-mapping rewrite.

### Callback/signature follow-up delivered with T0

`Func` values (named functions, lambdas, and initializer aliases) are distinct
from the values returned by calling them. Callback slots rank function values;
other argument slots retain invocation-return ranking. Scope-aware local hover
and argument checks use callable facts without executing bodies. Signature help
uses structured defaults and refuses to borrow same-named global contracts for
locals, including parameters/aliases/mutated bindings in Xeto and Trio bodies.

This is not callback signature/arity compatibility or local callable invocation
inference. `ioHttp` remains `Obj?` rather than adopting a callback's `Dict?`, and
function-body returns are not checked against declared return types. Local
definition/reference resolution and the broader F2/T1 shared facts work remain
deferred; signature guards do not establish a complete semantic snapshot.

## 4. T1: flow-sensitive reassignment inference

### 0.4.4 bounded accumulator fix (delivered separately)

The reported `results: []; recs.each(r => results = results.add(...))` case now
retains its List category. This is an invariant proof, **not** T1a–T1c: it starts
from a literal-backed initializer and checks all writes to the exact lexical
binding, regardless of callback scheduling or source order. List literals,
groups/aliases, and full calls to the selected pinned built-in `add` are the
supported List-producing expressions. Unknown/type-changing writes, lexical or
indexed overrides, partial application, and budget exhaustion do not establish
an invariant. Declared parameter types are not reinterpreted without qualified
identity resolution. Diagnostics and Xeto hover/completion consume the same
proof; no list element type, callback execution, or `each` result specialization
is inferred.

The [language-review notes](../type-intelligence.md#language-semantics-reviewed-for-this-patch)
record the architecture/namespace, closure, immutable-list, and dot-call semantics
checked against upstream documentation and pinned `AxonFuncs.fan` source. Broad
function returns remain unchanged in the cache. The planned transfers below
are still needed for changing types, per-position writes, and branch/effect joins.

The full user reproduction additionally exposed an F1 parser bug: optional-else
lookahead consumed the newline before `results: []` and recovery discarded the
declaration name. Preserving that boundary fixes the enclosing `if` case without
changing the List proof or interpreting `List<of:...>` as element-type inference.

### Target behavior

```axon
value: 1
inspect(value)       // Number at this point
value = "updated"
inspect(value)       // Str at this point
```

This must also work for separate statements on one line. Reads on the RHS see
the prior state; the new state becomes visible only after the RHS completes.
An unknown RHS remains unknown and must not retain the earlier initializer.

### Required model changes

1. Identify a lexical variable slot separately from each definition/write site.
   Nested shadowing gives a different binding identity. Multiple definition
   sites in exclusive branches need joins, not an arbitrary later-source winner.
2. Retain AST expressions and exact spans for initializer/write RHSs, target
   identities, owning function, and control-flow/deferred context. Do not store
   only the target spelling and a line number.
3. Keep name visibility/possible existence separate from definitely initialized
   state. A conditional definition may not have run; a closure may resolve an
   outer name defined later when it is eventually invoked.
4. Evaluate abstract facts, never code: literal categories, safe aliases, known
   resolved function returns, groups, and a small verified operator subset.
   Replace `strings.Split("+")` inference with AST-based operator dispatch.
5. Snapshot alias values at assignment time. After `a: 1; b: a; a = "x"`, `b`
   remains Number; a live edge that follows the current fact for `a` is wrong.
6. Preserve a parameter's declared call contract separately from its current
   observed value. Axon reassignment is not automatically a static type error.
   Do not continue presenting a reassigned parameter's entry type as its value.

### Phased change path

**T1a — Shared facts and straight-line writes (medium):**

- Migrate literal/return/local expression queries into the shared layer while
  preserving current diagnostics. Expose fact-at-position queries.
- Add ordered transfer for definitions and writes within one function and one
  straight-line path. Support simple aliases and resolved local callable values.
- Keep existing mutation invalidation at unsupported branch/closure boundaries
  until those transfers are implemented. Do not infer by the last textual write.
- Update hover, completion compatibility, and argument validation together.
  Stronger local-variable diagnostics are an explicit reviewed expansion, not
  an incidental consequence of exposing a more precise hover.

**T1b — Branches and abrupt completion (medium/large):**

- Analyze `if` branches from copies of the incoming state. Include the unchanged
  path when there is no `else`; merge presence and value facts at continuation.
- Preserve a fact when all reachable paths agree. `Unknown` joined with any
  fact remains unknown. A Number/null join is maybe-null, not definite null.
  Unions may be tracked internally if useful, without claiming every branch is
  incompatible when only one is.
- Model short-circuit `and`/`or`, `return`, and `throw` as execution structure.
  Do not let writes in a returning branch poison a continuation it cannot reach.
- Model try/catch from actual expression ownership. A caught exception may
  occur after only some writes in the try body. Catch parameters may update a
  current-frame variable: resolve the foundation plan's runtime compatibility
  question before implementing a catch join.
- Do not add whole-program CFG/SSA infrastructure unless structured AST transfer
  proves insufficient. Keep analysis work bounded per function/revision.

**T1c — Closures and effects (large; initially precision-limited):**

- Distinguish closure creation from invocation. Collect captured reads/writes;
  creating a lambda must not pretend its assignments executed immediately.
- Initially retain unknown/invalidation for potentially invoked captured writes
  instead of attempting general interprocedural evaluation or callback execution.
  Unknown higher-order calls can invoke or store a closure.
- Reads inside a closure depend on captured frame state at invocation, not only
  the outer initializer at the lambda's source location. Recursion, escaping
  closures, and dynamic/FFI calls remain conservative.
- Optional later effect summaries require identity/cycle/recursion budgets and
  independently reviewed semantics; do not expand T1a to include them.

### Acceptance matrix

| Case | Required result |
| --- | --- |
| Read before/after straight-line assignment, including same line | Correct fact at each read, no premature update |
| Unknown write | Unknown, never stale initializer type |
| Alias copied before source variable changes | Alias keeps the captured value category |
| Inner definition versus captured write | Only the correct binding's state is affected |
| Both branches agree / disagree / one absent | Preserve agreement; otherwise conservative joined fact and presence |
| Branch returns before later read | Only continuing paths contribute |
| Closure created but not invoked | No fabricated execution; conservative effects are distinguishable from known writes |
| Outer name defined after closure creation | Do not issue a definite undefined error based solely on textual order |
| Declared Number parameter reassigned to a different value | Declared contract and current value are not conflated |
| Incomplete RHS or unsupported expression | Unknown fact localized to the affected analysis region |

**Exit:** diagnostics, hover, and ranking share the same position-sensitive fact;
no interpreter/runtime is invoked; branch/closure uncertainty remains explicit.

## 5. T2: static trap-result typing

### Important dependency correction

T2 is not one small extension of `TrapExpr`. There are two useful slices:

- **T2a:** local literal shapes; depends on F1/F2 and shared type facts.
- **T2b:** resolved Xeto slots; additionally depends on T3a metadata/identity and
  a complete-enough active context. Full version solving is not required for a
  self-contained local fixture, but unresolved/ambiguous libraries must yield
  unknown rather than a guessed slot type.

### T2a — Local closed shapes (small/medium after shared facts)

```axon
rec: {curVal: 1, site: {dis: "HQ"}}
value: rec->curVal          // Number
label: rec->site->dis       // Str
```

- Build shape facts from complete map AST entries. Preserve literal key names,
  presence, markers/removes, duplicate-key handling, and value facts. F1's map
  grammar support is a prerequisite, not optional test cleanup.
- Distinguish closed literal shapes from generic Dicts, declared spec shapes,
  dynamic results, and data inferred from partial syntax.
- Carry trustworthy shapes through aliases and supported assignments. Drop or
  join them conservatively at unsupported effects/branches. Do not assume all
  possible foreign/dynamic objects have immutable record semantics.
- Handle chain composition without registering trap keys as ordinary function
  references. Keep filter paths out of value inference.
- First release provides type facts/hover/ranking only. Do not add a generic
  missing-tag diagnostic: runtime lookup, sparse/null tags, and open shapes have
  different behavior from a missing local variable.

### T2b — Named spec slots (medium/large)

1. Resolve a receiver TypeRef in its source namespace. Obtain its effective slot
   set with declared/base/mixin provenance and completeness information.
2. Resolve a trap key against those slots; return the selected slot's TypeRef,
   maybe/presence information, declaration source, and supporting library
   generation. Never look up a function just because it has the tag's name.
3. Compose nested Dict slots and typed Ref targets only where `of` metadata and
   namespace identity are understood. A bare Ref contains no referent shape.
   No `readById`, network/database access, Xeto build, or runtime evaluation is
   part of LSP analysis.
4. Separate type-on-success from a guaranteed successful lookup. `->` raises for
   absent/null tags; `[]` can return null. Do not silently model both as the same
   optional lookup or assert that a record exists. Retain conservative maybe
   facts until normal-path refinement rules are explicitly supported.
5. Decide whether slot hover/definition is in scope. If enabled, it targets the
   slot's metadata declaration, not a same-named function or a live data record.
   Existing function-reference behavior stays distinct.

### Acceptance and exit

- Literal nested shapes infer supported values; unknown/dynamic shapes do not.
- Named slots resolve uniquely in local, external, and archive providers, with
  inherited/mixin slots only when the effective model is complete.
- Same-named specs in different libraries cannot supply each other's slots.
- A Ref without target metadata, an unresolved dependency, or incomplete base
  yields unknown. Nullable receivers/tags do not produce speculative errors.
- Filter left paths and evaluated comparison values use different rules.
- Unsaved slot edits, source removal, archive rebuild/clean, and setting changes
  invalidate facts without stale members reappearing from a lower-priority copy.

**Exit:** no runtime record lookup; every known trap fact has inspectable static
evidence; unknown remains the default for incomplete or ambiguous metadata.

## 6. T3: split metadata, namespaces, and conformance

### Upstream evidence that constrains the design

Paths below are under the clean pinned 4.0.6 checkout.

- `src/core/xeto/fan/Spec.fan:35-93`: qualified names, own versus effective
  members, nominal `isa`, maybe types, and And/Or are separate concepts.
- `src/core/xeto/fan/Namespace.fan:12-30,124-158`: a namespace pins library
  versions; unqualified type lookup can be ambiguous; nominal and structural
  relations are not interchangeable.
- `src/core/axon/fan/AxonContext.fan:117-141`: unqualified functions also have
  ambiguity/overridable companion-library rules. Current source-root precedence
  is an editor policy, not a full implementation of runtime namespaces.
- `src/core/xeto/fan/LibDepend.fan:43-98`: versions use exact three-segment,
  `x`-wildcard, and inclusive range constraints. Do not use npm/caret SemVer
  assumptions. `xetom/fan/util/DependSolver.fan` describes a greedy latest-match
  strategy, not a full SAT solver.
- `src/xeto/hx.point/lib.xeto:9-17`: source manifests may contain unresolved
  `BuildVar` versions/dependencies. Path names alone are not library identities.
- `src/core/xetom/fan/util/Fitter.fan:39-67,73-132,147-241`: nominal spec fitting,
  structural spec fitting, and fitting a runtime Dict are different operations;
  effective mixins, maybe slots, globals, and distinctive matches matter.
- `Fitter.fan:216-235`: absent/null values for non-maybe slots fail; optional
  missing slots are skipped. A default expression must not be assumed to fill a
  missing value during fitting.
- `Fitter.fan:112-144` and `CheckVal.fan`: Haystack versus full fidelity affects
  Number subtypes and other values. Literal spelling is not enough to enforce
  Int/Float/Duration contracts or arbitrary enum/unit constraints soundly.

### T3a — Qualified metadata and coherent providers (large, enabling work)

**Likely files:** Xeto parsing/model types, index types/manager/archive lifecycle,
cache serialization/builder; new narrow metadata/query helpers if necessary.

1. Expand T0's narrow parsed document model alongside existing function parsing: declared
   specs, slots, base/type expressions, mixins, metadata, source spans, and a
   library manifest. Reuse quote/nesting-aware scans but do not attempt a spec
   tree with a top-level-name regex. Keep existing parser adapters while migrating.
2. Model library/provider identity separately from names/version constraints:
   editable tree, external tree, archive owner, bundled catalog; include origin,
   precedence, selected version or unresolved BuildVar, and content generation.
3. Represent own versus effective slots. Resolve bases/mixins with explicit
   missing/ambiguous/cyclic states and bounded traversal. Preserve `maybe`, `of`,
   `ofs`, inherited metadata, and unsupported constraints without interpreting
   unsupported forms as absent.
4. Choose a coherent library candidate before deriving effective types. Do not
   combine new workspace slots with stale archive slots from a lower-priority
   copy. Overlay unsaved files on their editable tree; a deleted slot must stay
   deleted. Multiple definitions in one selected candidate are not silently
   merged by lexical file order.
5. Provide a contextual spec/type query, not a replacement global `HasSpecName`
   boolean. Keep the existing coarse check as a compatibility fallback where
   no trustworthy namespace is available, explicitly marked as such.
6. Extend T0's additive, schema-versioned catalog for structural metadata, or
   choose a measured lazy parse of bundled Xeto sources. Prefer generated metadata for predictable
   startup; preserve old function-cache readers/defaults while migrating. Full
   sys/ph dependency coverage must be audited: source text shipping with the
   Haxall cache is not proof that every dependency is included.
7. Reuse archive parse limits, provider normalization, replacement cleanup, and
   settings behavior. Decide explicitly whether disabling bundled Xeto metadata
   also hides future bundled structural catalogs; recommendation: no hidden
   bundled semantic fallback when the user requested its exclusion.

**Exit:** self-contained qualified/local spec fixtures can resolve to complete
effective slot models; unresolved cases remain inspectable/unknown; source and
archive precedence cannot splice incompatible provider copies.

### T3b — Library/dependency/version contexts (large, separate delivery)

1. Parse library name/version/dependency declarations from supported manifests.
   Associate loose files and archive entries with their owning library; do not
   infer a version merely from an archive filename. Keep standalone legacy Trio
   workflows functional when no Xeto library context exists.
2. Separate discovery of candidates from selection of a namespace. Select at
   most one version per library within a context, while permitting different
   contexts for different roots/callers. Respect dependency constraints before
   using root priority to break ties between equivalent valid providers.
3. Implement documented exact/wildcard/range matching, transitive constraints,
   missing dependencies, conflicts, cycle detection, and deterministic output.
   Explicitly choose compatibility with the pinned greedy solver versus a
   stronger solver; do not silently add exponential search or claim runtime
   parity while choosing different versions.
4. Treat unresolved BuildVar/build properties as unavailable information unless
   a bounded, read-only, documented property-resolution subset is supported.
   Never execute build scripts or download missing libraries to resolve types.
5. Make a strict context query distinct from current global editor lookup.
   Retain current-file/workspace/external/bundled behavior as the default until
   the migration and configuration policy are approved. Ambiguous unqualified
   names in strict contexts must not pick a global winner by spelling.
6. Invalidate context/type facts on manifest/source edits, archive replacement,
   root reordering, cache settings, and close/revert. Publish clear context
   status rather than flooding every dependent call with the same error.

**Exit:** deterministic offline resolution, constraints are never bypassed by
priority, incomplete environments do not produce false missing-spec/type errors,
and existing non-strict function lookup retains documented precedence.

### T3c — Conservative structural conformance (large, separate delivery)

Do not implement this as `actualTypeString != expectedTypeString` with a longer
list of exceptions. Distinguish:

- nominal type assignability;
- whether a known value/closed literal shape satisfies a spec;
- whether one spec's contract structurally satisfies another;
- unknown because constraints, dependencies, values, or reference targets are
  unavailable. No definite success should be claimed after silently skipping
  required unsupported rules.

Change path:

1. Define a three-way relation (compatible, definitely incompatible, unknown)
   with evidence and constraint coverage. Unknown candidates remain visible in
   completion; only definite incompatibility produces a warning.
2. Start with complete, self-contained specs and closed literal Dict values:
   required/maybe slots, supported primitive slot types, and simple resolved
   inheritance. Missing required slots need complete effective metadata and
   definitely absent values. An open Dict cannot justify that warning.
3. Reuse T0's positive nominal inheritance and add safe structural mismatch evidence.
   Additional slots are not automatically forbidden; global-slot constraints
   and distinctive-match requirements must be accounted for before claiming a
   complete fit. Spec-to-spec fitting is not the same algorithm as value fitting.
4. Integrate with shared argument checks, using a precise slot/value range and
   provenance where available. Gate new structural warnings initially; do not
   convert today's unknown custom-spec values into blanket mismatches.
5. Add lists/`of`, refs with static target evidence, compound types, enums,
   units/ranges, globals, mixins, and constraints only as independently verified
   slices. Runtime graph/query checks, choices, dynamic FFI behavior, unresolved
   references, and unsupported metadata initially yield unknown.
6. Decide Haystack versus full fidelity before enforcing numeric subtypes. Keep
   function omission defaults, value nullability, optional record slots, and
   structural default/instantiation behavior separate.

**Exit:** every warning demonstrates a definite failing constraint; missing or
ambiguous metadata cannot manufacture failure or success. Compare selected
cases with pinned upstream behavior, not only with self-authored Go tests.

## 7. Dependency and delivery map

```text
T0 declared nominal identities/base graph (implemented independently)
  +-- receiver/argument compatibility
  +-- reusable subset of T3a metadata

F1/F2 reliable views, ASTs, roles, and binding identities
  |
  +-- shared type facts --> T1a --> T1b --> optional T1c precision
  |
  +-- shape facts --------> T2a
  |
  +-- T3a qualified metadata/effective slots --> T2b
                             |
                             +--> T3b dependency/version contexts
                             |
                             +--> T3c local bounded conformance
                                     (broader contexts require T3b)

F3 syntax diagnostics is independently gated; do not make it a reason to
delay safe type queries after F1/F2 contracts are established.
```

Suggested reviewable units: shared type facts; straight-line assignments;
branch transfer; literal shape/trap facts; declared spec/library metadata;
effective local slot resolution; contextual trap facts; offline dependency
selection; local structural checks; broader structural context support. Do not
bundle them all into the next release or change all index keys in one commit.

## 8. Test strategy, budgets, and review decisions

- Unit tests own transfer/join behavior, type relations, constraints, and graph
  resolution. LSP tests own agreement between features, host mapping, settings,
  and lifecycle invalidation. Keep source-precedence/archive tests intact.
- Reuse small explicit metadata fixtures rather than a large runtime Haxall
  installation in every test. Add a limited pinned-upstream conformance corpus
  with provenance/licensing reviewed; do not copy implementation code wholesale.
- Cap type/shape nesting, inheritance/dependency traversal, cyclic expression
  inference, issue counts, and snapshot storage. Memoize by identity and context
  generation, not unqualified names. Benchmark representative documents before
  adding caches or a more expensive dependency solver.
- Retain current race/lifecycle checks; add mutation/manifest/archive edits that
  remove facts as well as add them. A stable virtual URI must not retain old
  inferred slot data after rebuild.

Decisions for review:

1. Approve T1a as the next inference milestone after F1/F2, keeping branch/closure
   precision separate?
2. Deliver T2a literal-shape facts before investing in a full spec catalog?
3. Approve T3a's minimal metadata subset as a prerequisite for T2b, without
   claiming full Xeto compilation or dependency resolution?
4. Keep strict namespace selection and new structural warnings opt-in at first,
   preserving current lookup behavior by default?
5. Which dependency policy is required: pinned-runtime greedy compatibility,
   explicit project pins, or a stronger resolver with documented differences?
6. Confirm static/offline analysis only: no runtime/database connections,
   automatic library downloads, or build execution?

This record now distinguishes implemented T0 from the preserved T1–T3 proposals.
Those later proposals do not imply new protocol fields, settings, version bumps,
or release commitments.
