# Upstream Xeto and Haxall notes

This document collects potential improvements to Xeto and Haxall that would
benefit Axon LSP and other editor/tooling consumers. These are proposals, not
claims about current upstream behavior. The goal is to refine them into focused
issues or pull requests for the appropriate upstream repository.

## Xeto / Haxall: explicit function parameter defaults

Generated Xeto `Func` signatures should preserve Fantom parameter default
values explicitly. When a Fantom function is converted into Xeto metadata, its
default arguments should be represented in the generated Xeto signature rather
than being lost or inferred from prose. This would let runtimes and tooling
agree on which arguments may be omitted and what value omission supplies.

Likely implementation point: Haxall's Fantom-to-Xeto signature/spec generation
pipeline, with a corresponding Xeto representation if the format does not
already express all Fantom defaults.

## Xeto: explicit nullable and optional parameter metadata

Function signatures should unambiguously indicate whether an input accepts
`null` and whether the argument may be omitted. The desired behavior is that a
nullable input is also an optional argument, with its nullability/default
semantics explicit in the signature. If Xeto needs to distinguish “may be
null” from “may be omitted,” both properties should be representable rather
than conflated.

This metadata would improve call validation, completion details, generated
documentation, and language-server diagnostics without requiring tools to
guess from type strings or documentation.

## Xeto: editor-facing namespace and Spec resolution

Provide a stable API or metadata view for tooling to resolve unqualified Spec
names in the active library namespace, including the resulting qualified name
and source location. Tooling needs to distinguish a locally scoped variable
from an unqualified Spec and determine which Specs are visible from the
current namespace/dependencies.

This would support correct diagnostics, hover, definition, and reference
handling without each language server independently reimplementing library
visibility rules.

## Haxall / Axon: source-positioned parser or token API

Consider exposing a supported Axon tokenizer/parser API that returns token and
AST source spans, including for Axon embedded in Xeto or Trio. Stable source
locations would make diagnostics and editor features more reliable than
independent text scanners, and would benefit formatters, linters, and other
language tooling in addition to this LSP.

For editor use, the API should also support error-tolerant parsing and partial
ASTs while a user is typing, with stable node/token kinds and source maps from
embedded Axon fragments back to the containing Xeto/Trio document. Without
those guarantees, every editor tool must rebuild both recovery behavior and
host-offset mapping itself.

## Xeto: embedded Axon source-map contract

Expose the exact source range and host-to-embedded offset mapping for every
embedded Axon body. A stable API should account for indentation, delimiter
lines, and line-ending conventions, and map parser diagnostics back to the
original Xeto document. The same facility would help formatters and future
Xeto-aware tools.

## Xeto: explicit function receiver semantics

Expose whether a `Func` parameter is the implicit receiver for member/pipe
calls, rather than requiring tooling to infer receiver behavior from a
parameter name or call spelling. An explicit receiver role would let call
validation bind positional arguments consistently for ordinary and member
calls, and would improve signature help and generated documentation.

## Follow-up process

- Verify each proposal against current upstream behavior and terminology.
- Identify whether the change belongs in Xeto's data model, Haxall's
  Fantom-to-Xeto generation, or Haxall's Axon parser/runtime.
- Split proposals into small issues with concrete examples and compatibility
  expectations before preparing pull requests.
