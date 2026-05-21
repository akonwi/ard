# 0007: Use Explicit Checker Error Recovery

## Status

Accepted

## Context

The Ard checker reports semantic errors while building typed program information for later compiler phases. A naive `addError()` and `nil` return pattern can create cascading diagnostics: one root error causes later checks to fail because expected checked expressions, declarations, or types were missing.

Cascading diagnostics make errors harder to understand and can hide the real source of a problem. At the same time, the checker should not continue blindly after errors that make subsequent analysis unreliable.

## Decision

Use explicit checker error recovery based on the severity and recoverability of the error.

For recoverable local errors, the checker should:

- report the diagnostic
- return a placeholder checked value with a reasonable type or shape
- continue checking nearby code to find additional useful diagnostics

Examples of recoverable placeholders include unknown/`Any` types for failed type resolution, fallback literal values for invalid literals, and placeholder string chunks for interpolation failures.

For critical errors that make continued checking unreliable, the checker should:

- report the diagnostic
- mark checking as halted
- stop further expression/statement checking that would otherwise produce cascades

Undefined variables are a representative critical error because later references depending on the missing symbol usually create noise rather than useful diagnostics.

Iteration-style contexts, such as import processing or independent declaration lists, may skip the failed item and continue with the rest when that produces useful independent diagnostics.

## Consequences

- User-facing diagnostics should focus on root causes rather than cascades.
- The checker needs clear judgment about which errors are recoverable and which should halt checking.
- Placeholder checked values must be safe for later compiler phases and should not imply that invalid code is valid.
- Remaining recovery gaps can be improved incrementally without changing the overall strategy.
- Tests should document both recovered diagnostics and halted critical-error behavior.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
