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
- [x] Phase 2: Complete structured signatures and contextual resolution.
- [x] Phase 3: Call-aware signature help with active-parameter tracking.
- [x] Phase 4: Receiver and argument type-ranked completion.
- [~] Phase 5: Conservative argument mismatch diagnostics.
- [ ] Phase 6: Additional primitive and selected structural compatibility rules.

## Current Type Rules

Known literals include strings, booleans, numbers, `null`, lists, and dicts.
Function return signatures and enclosing function parameters may infer variable
types. Unknown expressions remain unknown.

Phase 1-3 currently covers embedded Xeto bodies, source-ordered local binding
lookup, nested and multiline call tracking, quote-aware active-argument
counting, and final-result inference for dotted call chains. Receiver-ranked
completion and argument diagnostics remain separate phases. Phase 4 now ranks
functions after a typed receiver or at a typed argument position, and includes
known compatible variables from the current embedded scope.

Phase 5 reports definite primitive literal mismatches only. Phase 6 includes
primitive literal categories and compatibility rules; structural Xeto slot
compatibility remains intentionally deferred because it requires stronger
semantic modeling than this diagnostic pass provides.

Phase 5 follow-ups are active: diagnostics must preserve complete expression
ranges, distinguish unresolved identifiers from unknown types, and report
missing required arguments only when signature metadata establishes them.

## Acceptance Fixtures

```axon
items: []
items.
text: "hello"
someFunction(
result: someFunction(text)
```

Tests must also cover nested calls, multiline calls, shadowing, reassignment,
and method-style calls.

## Decision Log

- Modern built-in signatures are collected from Xeto only; legacy Trio remains
  available as a separate cache source.
- Haxall 4.0.6 is pinned to commit `b79cdfa41a8f6ac9e5b3b1e321d1ed33e8d70f5f`.
