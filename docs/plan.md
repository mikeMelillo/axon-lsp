# Axon LSP: Forward Plan

This is a working plan for making the extension more dependable and then adding
focused language-authoring features. It is intentionally staged: syntax and
scope correctness come before broader linting, type intelligence, or automatic
formatting.

## Product direction

The goal is a practical editor for Axon, Xeto, and embedded Axon that reflects
Haxall's language behavior rather than approximating it from indentation or
isolated regular expressions. The server should prefer conservative results:
unknown syntax or types should not produce confident but incorrect warnings.

The Haxall [Axon Language](https://haxall.io/doc/hx.doc.haxall/AxonLang) page is
the initial language reference. In particular, Axon uses lexical function
scopes: `do`, `if`, and `try` blocks do not create ordinary variable scopes;
nested functions can capture outer variables; and indentation is not a general
scope terminator. Xeto and Trio still have their own outer document structures,
which delimit the Axon source regions to analyze.

## Current checkpoint — Xeto embedded-Axon scope slice

Implemented on 2026-09-27:

- Added a shared `internal/scope` model used by argument diagnostics and embedded
  Axon hover/type lookup.
- Seeded each embedded Xeto body with its owning `Func` parameters; added local
  declaration lookup, lambda parameter/local ranges, nested outer-binding
  visibility, catch variables, and parameter shadowing.
- Bounded lambda and catch bindings by `do`/`end` syntax rather than dedenting.
  A lambda in a declaration's initializer no longer shortens the outer
  declaration's lifetime.
- Stopped treating every colon-defined variable as a local function. Ordinary
  variables are now resolved through the scope model, preventing a binding from
  being accepted everywhere as a function/local name.
- Added regression coverage for the `targetEvents ... findAll(e => ...)` hover
  result (`Obj?`), variable visibility, indentation changes, closures,
  shadowing, and catch binding forms.

This is a focused static analysis, not a complete Axon parser or runtime
implementation. It currently analyzes each embedded Xeto function body as its
own enclosing function. Multiline expressions that omit explicit `do`/`end`,
full source-coordinate binding ranges, and equivalent scope analysis for Trio
function bodies remain follow-up work. Keep unknown results conservative and
extend the tests before broadening the parser's claims.

## Priorities

### 1. Reliable syntax and scope foundation — high priority, medium/large effort

Before expanding diagnostics, consolidate the current line-oriented scope
heuristics into a shared syntax-aware analysis layer.

- Keep source ranges in original document coordinates, including embedded Axon
  regions inside Xeto and Trio.
- Model enclosing function parameters, declarations, reassignment, nested
  lambdas, closures, and catch variables explicitly.
- Treat `do`/`end`, `if`, and `try` as expression/block structure, not automatic
  indentation-based scope boundaries. Give lambda parameters the lifetime of
  their lambda body, and make captured outer bindings available to nested
  functions.
- Support both `catch (e)` and `catch e` and scope the exception binding to the
  catch expression/body according to Axon behavior.
- Distinguish variables, function values, filter/tag expressions, and function
  calls. A known function such as `avg` may be passed as a value without `()`;
  a bare identifier in a `Filter`-typed parameter is not necessarily a variable.
- Share the analysis results across diagnostics, local hover/type lookup,
  signature help, and completions where practical. Avoid each feature inventing
  its own scoping rules.

**Acceptance cases:** Xeto `Func` parameters work throughout their embedded body;
variables declared inside a function remain available across `do`/`if` blocks;
lambda parameters work regardless of formatting indentation and do not leak
outside their lambda; closures can read enclosing variables; catch variables
work in both spellings; function values such as `avg` are accepted; and
`Filter`-typed arguments accept filter syntax without suppressing ordinary
undefined-variable checks elsewhere.

### 2. Diagnostic confidence and type intelligence — high priority, incremental

Build on the shared analysis layer rather than adding more independent
regex-based checks.

- Preserve the existing rule that uncertain types do not produce mismatch
  warnings.
- Continue conservative literal compatibility checks and distinguish a
  definitely incompatible value from an unknown expression.
- Track declared/inferred return types for complete outer expressions. For
  chained calls with lambdas, infer from the outermost call and ignore nested
  predicate calls when selecting the result type.
- Represent optional/default arguments accurately where the source metadata
  provides that information. Keep the `axonLsp.warnMissingArguments` setting as
  an escape hatch for incomplete or mismatched catalogs.
- Add a documented diagnostic catalog, severity policy, and settings only when
  each rule has stable behavior and regression fixtures.

### 3. Linting and syntax features — medium priority, incremental

Add opt-in or low-noise checks after parsing/scope behavior is dependable.

Potential lint groups:

- **Syntax structure:** unmatched `do`/`end`, malformed lambda/catch forms, and
  invalid constructs only when the parser can identify them confidently.
- **Name usage:** undefined functions and variables, with exclusions for
  comments, documentation, strings, filters, function values, and dynamically
  resolved names.
- **Call correctness:** definite primitive type mismatches and missing required
  arguments when metadata is trustworthy.
- **Code quality (later):** unused locals, suspicious shadowing, and simpler
  idioms. These should be separately configurable and should not block normal
  dynamic Axon patterns.

Do not turn uncertain analysis into errors. Prefer warnings that explain the
source of their information, and ensure every lint can be disabled or ignored
without disabling unrelated diagnostics.

### 4. Triple-quoted string support — medium priority

Axon triple-quoted strings have layout rules that ordinary quoted-string
highlighting does not capture. The Haxall language reference states that
triple-quoted strings may span lines, support backslash escapes, and require the
first non-whitespace character of each content line to align with the opening
quote's content position.

- Make syntax highlighting, comment masking, and source-range calculations agree
  on the complete triple-quoted string, including embedded quotes and escapes.
- Consider a syntax diagnostic for a definite indentation-rule violation only
  after the lexer can reliably distinguish string content from Axon code.
- Add fixtures for opening/closing delimiters, aligned and misaligned content,
  blank lines, escaped delimiters, and strings containing text that resembles
  comments or function calls.

Do not reindent the contents of a triple-quoted string as ordinary code.

### 5. Formatting — explore after syntax foundation, large effort

Consistent formatting would be useful, but an automatic formatter can silently
change program behavior or string contents. Treat it as a separate feature with
strict preservation requirements.

- First document a small style guide: whitespace around operators, call and
  lambda spacing, `do`/`end` layout, and Xeto/Trio embedded-source conventions.
- Prototype **range formatting** before whole-document formatting.
- Require idempotence (`format(format(source)) == format(source)`) and preserve
  comments, quote style, triple-string content, and Xeto/Trio wrapper syntax.
- Refuse or limit formatting for malformed/incomplete syntax rather than
  guessing.
- Use golden tests covering Axon, Trio, Xeto, nested blocks, lambdas, comments,
  and triple-quoted strings before exposing a format command.

Formatting should not be combined with lint auto-fixes until both have stable
source ranges and preservation tests.

### 6. Xeto function authoring templates — small, low-risk opportunity

Start with VS Code snippets rather than a code generator. A snippet is easy to
review, does not need server protocol changes, and preserves the author's
control over the Xeto source.

Candidate snippets:

- A minimal `+Funcs` container and `Func` declaration with parameter slots,
  `returns`, and an embedded `<axon:--- ... --->` body.
- A bodyless/spec-only function declaration for signatures implemented
  elsewhere.
- Common slot type choices and patterns: `Obj?`, `Bool`, `Str`, `Number`,
  `List`, `Dict`, `Filter`, and `Func`.
- A parameterized function template that makes optionality explicit and avoids
  implying that `?` and a runtime default are always the same concept.

Validate snippets against real Haxall Xeto syntax and ensure tab stops produce a
valid declaration. Avoid generating a large list of opinionated templates
before user feedback.

## Engineering workflow

- Keep a compact acceptance corpus of real Axon, Trio, and Xeto examples,
  including user-reported regressions.
- For each bug, add a focused test at the lowest useful layer and an LSP-level
  test when source mapping or configuration is involved.
- Test both positive and negative cases: a name must be recognized in its scope
  and must not leak into an unrelated scope.
- Keep parsing, semantic analysis, and LSP presentation separate so future
  syntax support does not require rewriting every feature.
- Preserve unknown as a first-class analysis result; do not convert it to
  `Obj`, `Bool`, or an undefined-name diagnostic merely to produce an answer.
- Defer debugger work until the current debugging path can be assessed
  independently.

## Suggested sequence

1. [x] Build the first shared scope-analysis slice for embedded Xeto Axon and
   lock it down with regression fixtures.
2. [ ] Extend the shared model to Trio function bodies and improve expression
   ranges/source mapping.
3. [ ] Revisit diagnostics and expression/type inference on top of the shared
   model, including remaining lambda and function-value cases.
4. [ ] Improve triple-string tokenization and add carefully scoped syntax
   warnings.
5. [ ] Add Xeto snippets as a small independent extension contribution.
6. [ ] Prototype formatting only after preservation and syntax tests are in
   place.

This sequence is directional, not a release commitment. User smoke tests and
real-world examples should reprioritize it as needed.
