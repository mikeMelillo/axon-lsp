# Implementation Plans

This is the entry point for implementation planning. The
[roadmap](../../ROADMAP.md) describes product direction;
[current analysis behavior](../type-intelligence.md) describes what is
implemented and its known limitations. [Upstream notes](../upstream_notes.md)
are separate proposals for Haxall/Xeto, not local implementation commitments.

## Active review plans

Baseline: 0.4.3 plus the pinned cache baseline (`2fc9630`). **T0 is implemented
along with the callback/signature correctness subset for the current 0.4.4 build.**
Broader F1–F3 and T1–T3 work remains draft for review, not approved release scopes
or claims that those proposed features already exist.

| ID | Change path | Detailed plan |
| --- | --- | --- |
| F1 | Regression corpus and error-tolerant parsing | [Analysis foundation](analysis-foundation.md) |
| F2 | Shared, position-aware resolution across editor features | [Analysis foundation](analysis-foundation.md) |
| F3 | Narrow, confidence-gated syntax diagnostics | [Analysis foundation](analysis-foundation.md) |
| T0 | Early declared nominal inheritance (implemented) | [Type-model evolution](type-model-evolution.md#3a-t0-early-declared-nominal-inheritance) |
| T1 | Flow-sensitive reassignment inference | [Type-model evolution](type-model-evolution.md) |
| T2 | Static trap-result typing | [Type-model evolution](type-model-evolution.md) |
| T3 | Structural conformance and library/version modeling | [Type-model evolution](type-model-evolution.md) |

### Implemented callback/signature subset

The [foundation plan's delivery record](analysis-foundation.md#1a-implemented-correctness-subset)
separates these fixes from the still-proposed F2 migration:

- Named functions, lambdas, and their initializer aliases carry `Func` facts;
  invocation return types remain distinct, with callback-aware completion/hover.
- Signature labels use structured types/defaults, plus a quote/nesting-aware
  legacy fallback; rendered commas no longer create phantom parameters.
- Signature help checks the actual callee's lexical binding in Xeto bodies and
  isolated Trio source records, withholding unrelated globals for local callbacks.
- Chained lambda initializers retain their bindings across supported `try/catch`
  bodies; leading-dot continuation lines retain their implicit method receivers.
  Indexed-function `Func` tooltips are shared across host formats.
- A bounded List-invariant proof retains literal-backed captured accumulators
  across built-in `add` assignments when all writes preserve List. Diagnostics
  and Xeto hover/completion share it; unknown writes remain conservative.
- Optional-`else` lookahead no longer consumes the next statement boundary when
  no else exists. The full `firstPassPipeline` regression retains `results: []`
  after its preceding `if`, including empty/populated List live edits.

Full local signature/result inference, callback signature conformance,
higher-order return propagation, declared-return checking, and local
definition/reference identity remain deferred. Do not describe these focused
fixes as completion of F2 or implementation of general T1/structural fitting.

### Recommended order

T0 was prioritized ahead of the broader foundation/type projects. It supplies
qualified nominal identities and positive ancestry queries without waiting for
flow analysis, effective slots, structural fitting, or version solving. Do not
make those projects prerequisites for using the declared base chain.

Continue with source-region/coordinate correctness and consistent name resolution,
with focused parser regressions. Do not publish the current parser error list
wholesale. The foundation plan owns the delivery slices, compatibility
decisions, and diagnostic rollout gates.

Add type inference after features share binding identities and distinguish
incomplete/unsupported syntax from definite errors. Literal-shape trap typing
can stand alone; named-slot typing needs qualified spec metadata. Library
resolution and structural checking remain separate deliveries. The type-model
plan owns their detailed dependencies and acceptance criteria.

The plans record reproduced LSP behavior and pinned-upstream comparisons, not
just desired features. Review their open decisions before scheduling work.

## Deferred authoring backlog

These ideas are preserved from earlier planning, but are not part of the active
foundation/type-model scope. User fixtures and feedback may reprioritize them.

### Triple-string authoring

Whole-fragment string/comment correctness belongs to F1. Beyond that, align
TextMate highlighting with lexical boundaries and verify the pinned language's
triple-string layout/alignment rules before adding layout warnings. Cover
aligned/misaligned content, blank lines, embedded quotes, escapes, and text that
resembles code/comments. Never reindent string contents as ordinary code.

### Formatting

Explore only after syntax/source preservation is dependable. Define a small
style guide for operators, calls, lambdas, blocks, and host-format conventions;
prototype range formatting before whole-document formatting. Require:

- Idempotence: `format(format(source)) == format(source)`.
- Preservation of comments, quote style, triple-string content, and Trio/Xeto
  wrapper syntax.
- Refusal or explicitly limited behavior for malformed/incomplete input.
- Golden fixtures for Axon, both host formats, nested blocks, lambdas, comments,
  and multiline strings before exposing a command.

Keep formatting separate from lint auto-fixes until both have reliable ranges
and preservation tests.

### Xeto authoring snippets

Start with a small set of VS Code snippets, not a code generator: a `+Funcs`
container, a `Func` with parameter/return slots and embedded Axon, and a bodyless
signature. Use a few common type choices (`Obj?`, `Bool`, `Str`, `Number`,
`List`, `Dict`, `Filter`, `Func`). Keep nullability and runtime defaults
distinct. Validate syntax/tab stops against real source and expand only after
feedback rather than shipping a large opinionated template catalog.

### Optional code-quality linting

Consider unused locals, suspicious shadowing, and simpler idioms after
resolution is reliable. Keep rules separately controllable and low-noise; do
not penalize dynamic Axon patterns. Each rule needs a documented diagnostic
code/severity and a way to ignore it without disabling unrelated checks.

### Debugging

Assess the existing development/debugging path independently before proposing a
language debugger. Do not bundle debugger work into parser or type inference.

## Planning principles

Reflect the supported language/runtime rather than indentation or isolated
regexes. Keep parsing, semantic analysis, and LSP presentation separate; unknown
must not become `Obj`, null, or an undefined-name warning merely to produce an
answer. Use compact real-world regressions at the lowest useful test layer,
with host/LSP tests where mapping, configuration, or lifecycle is the contract.

Detailed plans own implementation checklists. This index owns priorities and
deferred ideas; it does not maintain a second competing phase checklist.
