# Axon Type Intelligence

## Scope

Type intelligence initially targets embedded Axon bodies in `.xeto` files. The
analysis is conservative: declared, inferred, and expected types remain distinct;
unknown information does not produce a mismatch warning.

## Decisions

- Keep `coreFuncs.trio` for legacy signatures and use the pinned Haxall 4.0.6
  Xeto catalog for modern signatures.
- Select modern signatures for Xeto documents in automatic mode; retain explicit
  `defs` and `specs` mode behavior.
- Use standard LSP signature help rather than ghost text.
- Rank completions by known compatibility, but do not hide unknown candidates.
- Warn only on definite argument incompatibility; structural Xeto conformance is
  deferred until its rules are implemented.

## Phases

- [x] Phase 0: Record baseline, cache provenance, and selection policy.
- [x] Phase 1: Shared token-aware analysis with scoped, source-ordered bindings.
- [x] Phase 2: Structured names, types, returns, explicit defaults, and contextual resolution. Older bundled catalogs may lack default metadata.
- [x] Phase 3: Call-aware signature help with active-parameter tracking.
- [x] Phase 4: Receiver and argument type-ranked completion.
- [~] Phase 5: Conservative argument mismatch diagnostics.
- [ ] Phase 6: Additional primitive and selected structural compatibility rules.

## Current Type Rules

Known literals include strings, booleans, numbers, dates, times, datetimes,
date spans, `null`, lists, and dicts.
Function return signatures and enclosing function parameters may infer variable
types. Unknown expressions remain unknown.

Phase 1-3 currently covers embedded Xeto bodies, source-ordered local binding
lookup, nested and multiline call tracking, quote-aware active-argument
counting, and final-result inference for dotted call chains. Receiver-ranked
completion and argument diagnostics remain separate phases. Phase 4 now ranks
functions after a typed receiver or at a typed argument position, and includes
known compatible variables from the current embedded scope.

Phase 5 reports definite primitive argument incompatibilities using literal
types and available function-return metadata. Additional compatibility rules
and structural Xeto slot checks remain deferred to Phase 6 because they require
stronger semantic modeling than this diagnostic pass provides.

Phase 5 preserves source-spanned argument ranges, distinguishes unresolved
receivers/simple argument identifiers from unknown types, and reports missing
required arguments only when signature metadata establishes them. General
identifier validation and flow-sensitive inference remain follow-ups.

Dot calls with and without parentheses validate their receiver as the first
argument. Explicit Xeto `axon` default metadata permits omission; default
presence is separate from nullable type metadata and is preserved through
source/archive indexing and cache serialization. Nullable function returns
are not treated as definite null values.

Trap expressions (`rec->tag`) have their own AST node and primary/postfix
precedence. Tag keys are literal names, not dot-call functions or variable
references. Receivers are validated except where signature metadata establishes
filter-path syntax. Trap results remain unknown: no runtime record resolution,
tag-existence checks, or structural slot-type inference is attempted.

Scope analysis uses source-spanned definitions and lexical function scopes.
`:` declares; `=` resolves an existing binding without creating a new one.
Position-aware diagnostics distinguish same-line definition/use order. Parameters
and captured bindings may be reassigned, and nested definitions remain local.
After a reassignment, value-type inference conservatively returns unknown rather
than reusing stale initializer or parameter type information. Complete
position-aware editor inference and control-flow analysis remain deferred.

## Acceptance Fixtures

```axon
items: []
items.
text: "hello"
someFunction(
result: someFunction(text)
```

Tests must also cover nested calls, multiline calls, shadowing, reassignment,
method-style calls, trap chains, filter paths, and incomplete-arrow recovery.
Host integration tests verify Trio/Xeto requests and UTF-16 diagnostic mapping;
AST shape and scope-ordering combinations belong in focused unit tests.

The 0.4.3 changes passed the full Go test suite, race detection, vet, client
watcher tests, and TypeScript compilation. Manual VS Code smoke testing was
also reported passing. These results validate the current conservative rules,
not the deferred flow-sensitive or structural analysis.

## Decision Log

- Modern built-in signatures are collected from Xeto only; legacy Trio remains
  available as a separate cache source.
- Haxall 4.0.6 is pinned to commit `b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f`.
