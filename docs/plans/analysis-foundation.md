# Analysis Foundation: Recovery, Resolution, and Syntax Diagnostics

Status: **draft for review; no implementation authorized by this document**.

Analysis date: 2026-10-16. Repository baseline: `e814e54` (0.4.3).
Companion: [type-model evolution](type-model-evolution.md).
Overview: [planning index](README.md).

## 1. Scope and recommendation

This document develops the three near-term items from the discussion:

- **F1:** establish focused regressions and improve parser recovery.
- **F2:** reduce competing analysis paths and make editor resolution consistent.
- **F3:** introduce syntax diagnostics only for supported, high-confidence cases.

F1 is a testing/recovery workstream throughout F2 and F3, not a proposal to land
a large suite of knowingly failing tests. Pair each new failing regression with
its behavior fix; keep unresolved cases in this plan until scheduled.

The important prerequisite is **one interpretation of a source occurrence**,
not a larger list of type rules. Do not combine these changes with structural
Xeto fitting, a complete compiler, formatting, or a new archive watcher system.

Recommended order: coordinate/region contracts -> valid grammar and local
recovery -> shared binding/occurrence resolution -> consumer migration -> a
small syntax-diagnostic release. Avoid a flag-day rewrite.

## 2. Evidence and baseline

### Method

- Read the Go lexer/parser, scope model, validator, LSP handlers, Trio/Xeto
  extraction, index selection, signature construction, and existing tests.
- Inspected the clean cached Haxall 4.0.6 checkout, whose HEAD is
  `b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f`.
- Ran in-memory JSON-RPC probes against `go run ./cmd/axon-lsp`; no probe files,
  fixtures, runtime code, settings, or cache assets were edited.
- Reran `go test ./...` from `go-server/`: passed. Existing passing tests do not
  cover the gaps below. No new manual editor smoke test was performed here.

### Reproduced through the LSP

For the following probes, open a signature Xeto document containing:

```xeto
+Funcs {
  planNeed: Func { value: Number }
  planFilter: Func { filter: Filter }
  planPass: Func { value: Obj }
  planDefault: Func { value: List <axon:"[1, 2]"> }
}
```

Unless stated otherwise, place each example in a separate Xeto `Func`'s
multiline Axon body. These are observed baseline results, not desired behavior.

| ID | Input/request | Observed result | Required direction |
| --- | --- | --- | --- |
| R1 | `planNoSuch()` versus `planNoSuch ()` | Only the first reports an undefined function | Resolve direct calls from AST/token roles, not byte adjacency to `(` |
| R2 | `planNeed: x => x` followed by `planNeed("text")` | Applies the global Number signature to the local lambda | Resolve the callable binding before selecting global metadata |
| R3 | `f: (planArg) => planArg` followed by `planPass(planArg)` | Correctly reports the out-of-scope parameter | Preserve this result under unrelated parse errors |
| R4 | R3 plus an unrelated final `rec->` | The undefined-parameter warning disappears | Do not re-enable document-wide heuristic parameter ranges on any parse error |
| R5 | `f: () => do ... inner: 1 ... end`, then completion at `planNeed(` | Offers `inner` after its function has ended | Enumerate visible bindings at the actual cursor, not at each declaration's line |
| R6 | `planFilter(siteRef->dis == planAbsent->dis)` | Does not report `planAbsent` | Only the left operand is a filter path; the right operand is evaluated Axon |
| R7 | `v: "😀"; planNoSuch()` | Undefined-function range is columns 11..21, not UTF-16 9..19 | Convert byte ranges and cursor positions at explicit boundaries |
| R8 | Triple-quoted text with `planNoSuch()` on its second line | Reports an undefined function inside the string | Use whole-fragment lexical context for every consumer |
| R9 | `text: "catch (planErr) do"` then `planPass(planErr)` | No undefined-variable warning | Catch-pattern matches inside strings must not create bindings |
| R10 | `planPass("oops)` | No syntax diagnostic | Add lexical completion/error state before attempting syntax publication |
| R11 | References on `planPass` at host line 3, body-relative line 0 | Reference location is returned at line 0 | Map references as well as diagnostics into host coordinates |
| R12 | Signature help for `planDefault(` | Produces two parameters from the single label `value: List = [1, 2]` | Build parameter labels from structured metadata, never split rendered signatures on commas |

R10 alone is not evidence that on-type EOF warnings should be enabled. It
demonstrates that there is currently no published syntax layer and no lexical
termination marker on the string token.

### Implementation findings from code inspection

Line anchors below refer to the baseline and will move during implementation.

| Location | Current behavior and consequence |
| --- | --- |
| `internal/diag/validator.go:44-106` | Parses once, then scope analysis reparses; mixes line-scanned functions/references with AST checks; any parse error enables heuristic parameter scopes |
| `internal/diag/validator.go:116-143` | Marks every trap in a Filter argument's subtree exempt, including evaluated comparison values and nested runtime expressions |
| `internal/diag/validator.go:180-251` | Uses global function metadata even when a lexical callable shadows it; incomplete calls may already produce missing-argument warnings |
| `internal/diag/validator.go:285-319` | Argument typing has literals/global returns but no shared local-binding type evaluator |
| `internal/scope/scope.go:51-121` | AST declarations/lambdas coexist with regex catch recovery; catch names lack AST declaration spans |
| `internal/scope/scope.go:140-183` | Exact-position resolution exists, but the editor-facing `Resolve` queries the end of a line; assignments retain name/position, not RHS or execution context |
| `internal/lsp/server.go:332-370` | Completion uses byte slicing/last-dot detection and scans declarations independently of the visible scope at the request position |
| `internal/lsp/server.go:603-734` | Most definition/reference lookups are word/global-function lookups; Trio signature context is limited to the current line |
| `internal/lsp/server.go:867-950` | Embedded hover is more scope-aware, but uses line-end lookup and repeated parsing; expression `+` inference still splits strings |
| `internal/lsp/server.go:819-848,1169-1250` | Trio diagnostics analyze the host document; Xeto first runs that path then replaces diagnostics with per-body results; diagnostic mapping does not remap reference side effects |
| `internal/trio/parser.go:136-164` | Existing extraction includes the `src:` field label and is used for block features, not a uniform semantic source view |
| `internal/xeto/parser.go:25-29,136-176,230-250` | Embedded regions have line offsets only; quoted `<axon:"...">` bodies are not extracted; incomplete host structure can lose region ownership |
| `internal/resolver/word.go:19-87` | ASCII regex byte ranges meet UTF-16 request positions; trap-key exclusion is line-local rather than a semantic occurrence classification |
| `internal/lexer/lexer.go:18-112` | A second lexer/masker resets quote state on newlines, does not model triple/raw strings consistently, and interprets comment columns as bytes |
| `internal/index/manager.go:66-83,386-392,399-412` | References are name-keyed and assembled as validation side effects; parameter presentation reparses `ArgsStr` |

Other audit targets: `literalAt` reparses single lines, block navigation uses
`internal/parser/blocks.go` and the legacy tokenizer, and code-action insertion
uses `len(line)` as an LSP column. Fix these coordinates within the same source
contract rather than making unrelated feature rewrites.

## 3. Upstream constraints before expanding diagnostics

Reference paths below are under
`cache_sources/haxall-4.0.6/src/core/axon/fan/`.

### Valid grammar the current AST does not fully represent

- `parser/Parser.fan:305-355`: `{point}`, `{-point}`, quoted keys, and keyword
  dict keys are valid; map keys are names, not evaluated variable expressions.
- `parser/Parser.fan:664-666,807-829`: general trailing lambdas and parameters
  with default expressions, including nested expressions, are valid. Current
  lookahead accepts only simple parameter names on one line.
- `parser/Parser.fan:648-652`: `_` marks partial application, not an undefined
  ordinary variable. Represent argument holes without compacting positions.
- `parser/Parser.fan:254-269`: `try <expr> catch [(name)] <expr>` does not require
  an additional outer `end`. A `do` block may omit its `end` before `catch` or
  `else` (`:129-145`). The current Go `parseTry` and `parseBlock` differ.
- `parser/Parser.fan:413-436`: range expressions are parsed inside comparison
  operands. The Go parser puts `..` and comparisons in the same precedence
  bucket; verify `x == 1..3` rather than blindly copying a precedence table.
- Top names, static/FFI calls, symbols, unit-bearing numbers, and legacy
  `defcomp` need an explicit supported/unsupported classification. `defcomp` is
  deliberately unsupported by Haxall 4.x but remains an LSP legacy feature.

This is not a commitment to implement every language form before F2. F3 must
not report the parser's lack of support for a valid form as invalid user code.

### Two semantic decisions that must not be hidden by a refactor

1. **Filter expression roles.** `parser/ExprToFilter.fan:65-95,106-143` converts
   comparison left operands and `not` operands to tag paths, but evaluates a
   comparison's right operand. A generic Filter-typed subtree is not a blanket
   exception to name checks. Also distinguish implicit dot receivers from lazy
   direct Filter arguments: `ast/Call.fan:96-110` evaluates the dot receiver
   before passing it to the target function. Current tests accept some bare
   tag dot receivers; preserving or correcting that compatibility requires a
   deliberate decision, not an accidental behavior change.
2. **Catch variable lifetime.** `ast/Operators.fan:193-221` calls
   `AxonContext.defOrAssign`; `AxonContext.fan:314-329` stores the result in the
   current frame. The same pattern is present in cached 3.1.12. Current Go tests
   instead insist that the name disappears after the catch body. Treat the
   post-catch value as conditionally established/updated, not automatically
   undefined. Verify runtime examples before changing tests or policy. The
   unparenthesized `catch e` spelling is current editor compatibility, not the
   pinned parser's documented catch-variable production.

Closures capture frames by reference (`AxonContext.fan:289-307`). A reference to
an outer variable defined after the lambda's source location may be valid when
the lambda is invoked later. Static lexical identity, definite initialization,
and source order of execution are different questions. F2 must not turn every
forward closure reference into a definite undefined-name error.

## 4. Proposed shared contracts

These are design sketches for review, not fixed new public APIs.

### Source view and coordinates

Give every analyzable body a view containing host URI, stable region identity,
owner metadata, original Axon text, byte spans, and bidirectional mapping between
fragment offsets and host UTF-16 positions. All ranges are half-open.

- For Trio, exclude record headers, documentation, separators, and the `src:`
  label. Account for inline source on the same line as `src:` and preserve or
  explicitly map continuation indentation. Each record/body is isolated.
- For Xeto, keep the owning function's parameters attached to the body. Map
  incomplete delimiters conservatively; do not consume the next function.
- Quoted `<axon:"...">` needs escape-aware mapping, not just an added line
  offset. It may be a later F1/F2 slice; until supported, do not diagnose its
  decoded contents as if they had direct host coordinates.
- Use one whole-fragment lexical view for strings/comments/raw/triple strings.
  Do not feed an Axon-only masker blindly across arbitrary Trio/Xeto host text.
- Centralize byte <-> UTF-16 conversion for cursor hit tests, references,
  diagnostics, completions, quick fixes, and block ranges. Preserve tabs and
  test LF/CRLF. Decide explicitly whether lone CR is accepted editor input.

A small neutral `internal/source` package is one possible home for mapping
types. Host parsers may construct views; that package must not import those
parsers back and create a cycle. LSP orchestration selects the host extractor.

### Analysis snapshot

Build a fragment snapshot once per revision: tokens, AST, issues, recovery
regions, scope/binding model, and source occurrences. Scope should accept the
already-parsed tree rather than independently parsing masked text again.

Initially reuse the snapshot within a request/diagnostic pass. Add cross-request
caching only after correctness tests exist. The current protocol structs do not
retain LSP document versions; use an explicit revision counter or preserve the
version field, rather than keying by URI alone. Separate syntax revision from
index/settings generation for semantic invalidation. Stable archive URIs do
not mean stable archive contents. Close, source replacement, configuration
changes, and archive reconciliation must invalidate the appropriate results.

### Name roles and resolution

Classify each occurrence: declaration, read, write target, direct/dot callee,
function value, literal trap/dict key, filter path, spec/top-name reference,
unsupported occurrence, or unresolved name.

- Resolve a lexical binding first; only consult the existing URI-aware function
  catalog when the language role permits global lookup.
- Give bindings stable identities within a fragment revision. Give global
  results selected-provider identity, not just spelling. Shadowing a global
  function must not inherit that function's parameters, return type, definition,
  or references. A local value with unknown callable type remains unknown.
- Preserve current global provider precedence, metadata modes, and bundled
  cache settings. F2 is not library/version resolution.
- Represent scope/name existence separately from definite execution. Catch and
  conditional definitions may be possibly bound. Defer a warning when a closure
  or unsupported fragment prevents a definite conclusion.
- Scope recovery fallbacks to a damaged span/region and label their confidence.
  Do not merge heuristic declarations into the entire document's lexical scope.

A proposed `internal/analysis` layer could depend on `axon`, `scope`, mapping
types, and a narrow catalog interface. Keep LSP/index adapters outside the
semantic core: `index` currently owns parsing/catalogs and must not acquire an
import cycle through a validator that already imports it.

### Occurrences are not diagnostic side effects

Produce resolved occurrences and semantic issues independently. Map all emitted
locations into host space before atomically replacing that document's reference
contribution. Disabling warnings must not leave references stale or prevent
their construction. Damaged regions must not retain deleted occurrences.

Initially preserve the existing function-reference product scope; this does not
promise a new complete workspace-wide reference index. Local navigation should
either use the binding's declaration span or return no global result, never
jump to a same-named global. Full local-reference UI is a separate review choice.

## 5. F1 change path: grammar/recovery and compact coverage

**Primary files:** `internal/axon/{lexer,parser}.go`, their tests,
`internal/lexer/lexer.go`, `internal/scope/scope.go`,
`internal/trio/parser.go`, `internal/xeto/parser.go`, LSP region handling.

1. Introduce lexical completion/error metadata. Unterminated strings, raw/triple
   strings, comments, and invalid tokens must not masquerade as complete
   literals. Preserve valid tokens and source positions after local damage.
2. Resolve disagreement between the legacy masker and Axon lexer. Start with
   Axon-fragment consumers; audit host masking separately. Retain `//lspignore`
   detection only in real line comments, never in a string.
3. Represent lambda parameter names/spans/default expressions, catch binding
   spans, partial-call holes, and dict key/marker/remove entries explicitly.
   Extend `Children` with the appropriate semantic children, not literal keys.
   Defaults are parsed but never executed by the LSP.
4. Correct try/catch and optional block-end handling against supported language
   versions. Preserve a compatibility decision for `catch e`; audit existing
   fixtures that require nonstandard extra `end` tokens.
5. Add general trailing-lambda parsing without treating it as a second call.
   Repair range/comparison grouping based on pinned grammar, with AST assertions.
6. Add explicit incomplete/error nodes or equivalent recovery metadata. Keep
   argument slots and separators, damaged spans, and the last trustworthy owner.
   Recovery synchronizes at delimiters/statement or host-region boundaries;
   never consume an outer `end`, `catch`, or next record as an inner construct.
7. Bound parser progress, nesting, issue count, and recovery work. An unfinished
   call cannot reliably distinguish every following newline from an argument;
   ambiguous recovery should retain uncertain nodes, not claim a definite AST.

**Coverage contract:** focused tables for valid AST shapes, invalid/incomplete
edits, and recovery preservation; scope tests for identity/lifetime; a small
Trio/Xeto integration matrix for mappings and request behavior. Add fuzz/property
checks for progress/no panic, valid span bounds, bounded recursion/recovery, and
source-map round trips. Do not multiply every unit case across all LSP features.

Valid corpus must include defaulted/nested/multiline lambdas, trailing lambdas,
partial application, dict markers/removes, keyword/quoted keys, try/catch forms,
branch end elision, traps/filters, refs/top names, units, and legacy defcomp.
Unsupported valid constructs are tracked as unsupported, not successful parses
or definite user errors.

**Exit:** valid supported forms retain structure without errors; damaged syntax
does not grant new bindings or suppress checks in unrelated complete regions;
mapping properties hold across comments, multiline strings, emoji, and CRLF.

## 6. F2 change path: migrate features to the shared interpretation

**Primary files:** validator, scope, resolver, index reference/signature methods,
LSP completion/hover/definition/signature handlers, protocol/source-map helpers.

1. Integrate source views and parse/scope reuse into both host paths. Remove the
   discarded whole-Xeto diagnostic pass. Analyze Trio source regions rather
   than treating record metadata as executable Axon.
2. Replace global `localFuncs` exemptions with binding-aware resolution in
   complete regions. Keep damaged/legacy recovery bounded. Resolve higher-order
   callable values before using a global signature; unknown does not mean error.
3. Model Filter path/value roles according to `ExprToFilter`, including nested
   comparisons and ordinary calls on evaluated operands. Do not carry path
   exemptions into lambdas or arbitrary runtime subexpressions.
4. Move direct-call undefined checks and function occurrences out of word scans.
   Use token/AST ranges for whitespace-separated calls and Unicode. Preserve
   unknown-callee receiver checks and existing ignore/configuration behavior.
5. Feed hover, local definition selection, completion binding enumeration, and
   signature help from the same position-aware snapshot. Use full-region call
   context for Trio as well as Xeto; use parsed receiver expressions for
   completion, not `LastIndex(".")`. Track commas/argument slots even when a
   cursor is before/after a missing or partial argument.
6. Build signature parameters from `Params`, type/default fields, and parameter
   spans where available. Retain a quote/nesting-aware fallback for legacy
   signatures lacking structured metadata; never split display text naively.
7. Replace document references atomically with mapped occurrences. Distinguish
   same-spelled locals/globals and selected global providers; preserve supported
   function-value references without registering literal tags or text content.
8. Remove a legacy scanner only after its valid/recovery cases have migrated.
   Mark retained fallbacks explicitly; do not leave two authoritative answers.

**Exit:** R1-R9/R11-R12 have corrected expectations; requests agree on an
occurrence's identity; no out-of-scope completion candidates; reference and
diagnostic coordinates share the same mapping; provider-precedence/archive
lifecycle tests remain unchanged in meaning.

## 7. F3 change path: confidence-gated syntax diagnostics

**Prerequisite:** supported grammar and host extraction must be sufficient for
the acceptance corpus. Publishing `ParseResult.Errors` directly is rejected.

1. Add stable issue codes/categories, context/expected-token information,
   incomplete-versus-invalid status, and recovery-origin spans. Parse issues are
   useful internally before any UI changes.
2. Classify nodes/regions as supported-complete, incomplete, recovered-invalid,
   or unsupported. Use that evidence to suppress dependent semantic cascades
   only in damaged regions. An unrelated valid statement is still checked.
3. Define a small diagnostic catalog: clear lexical failures and unmatched or
   mismatched closing delimiters in supported regions are initial candidates.
   Unexpected `end` requires grammar-aware ownership, not just block pairing.
   Defer EOF-only missing closers, incomplete arrows/calls, and unsupported host
   syntax while typing. Do not diagnose FFI assignment targets as invalid merely
   because only ordinary variable assignment is statically modeled.
4. Propose one opt-in conservative syntax-diagnostic setting, default **off** in
   the initial release; confirm the name/rollout at review. Do not add separate
   settings for every token error. Syntax and missing-argument warnings remain
   independently controllable. No initial timer-based heuristics or broad
   save-versus-type mode matrix.
5. Add optional diagnostic `code`/`source` fields to `index.Diagnostic`; preserve
   them in host mapping. Keep existing semantic severities/messages stable
   unless explicitly reviewed. Update quick-fix matching to prefer codes with
   compatibility fallback, and use correct UTF-16 insertion positions.
6. Emit a bounded primary diagnostic per recovery cause; do not emit both an
   unmatched delimiter and multiple fabricated undefined-name/missing-argument
   errors caused by its damaged subtree. Respect `//lspignore` on real comments.

**Exit:** no syntax warnings for valid/unsupported fixtures or incomplete-arrow
editing states under the initial policy; intentional invalid complete examples
produce stable codes, bounded diagnostics, and exact host ranges. Toggling the
setting refreshes open diagnostics without changing resolution/reference data.

## 8. Proposed delivery slices and review gates

| Slice | Scope | Gate |
| --- | --- | --- |
| A | Source views, host isolation, byte/UTF-16 conversions; R7/R11 | Review inline Trio mapping and incomplete-region ownership |
| B | Whole-fragment lexical context, localized recovery, valid grammar; R4/R8/R9 | Review catch lifetime/spelling, dot Filter receiver behavior, legacy support |
| C | Reusable AST/scope snapshot, roles, callable resolution; R1/R2/R6 | Confirm import boundaries and fallback confidence policy |
| D | Editor/occurrence migration, signature labels; R5/R12 | Confirm local navigation/reference product scope and invalidation |
| E | Coded issue catalog and opt-in syntax diagnostics | Approve initial rules, severity, and rollout; only then touch client settings |

F1 fixtures accompany each slice; this ordering can be split into smaller
commits. Do not land failing tests or broad ignored suites as a substitute for
a migration. Existing archive lifecycle/precedence tests protect different
contracts and should not be removed as apparent duplicates.

For each slice: targeted tests, `go test ./...`, and `go vet ./...`; run race
checks at the checkpoint. Client tests/compilation are required when settings or
client payloads change. New editor smoke cases should exercise valid -> partial
edit -> corrected transitions, Unicode positions, multiple Trio records/Xeto
bodies, shadowed function names, and signature help around nested/defaulted calls.

## 9. Decisions requested before implementation

1. Approve A-D as correctness work before exposing new syntax warnings?
2. Prefer pinned runtime behavior for catch lifetime and evaluated Filter
   receivers, retaining `catch e` only as an explicit editor compatibility form?
3. Keep F3 opt-in/default-off until real-user fixtures demonstrate low noise?
4. Add local definition navigation in D, or only stop false global navigation
   and defer new local-reference UI?
5. Include quoted Xeto Axon bodies in A/B, or record them as unsupported and
   deliver escape-aware mapping as its own follow-up slice?

No runtime changes, fixture files, caches, package versions, or release notes
are part of this planning-only change.
