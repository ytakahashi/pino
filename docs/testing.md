# Testing

## Test kinds

- A unit test covers one production file and lives beside it.
- A layer test covers several functions in a layer, or a layer and the ports
  beneath it. Name it after its subject rather than a production file. Double
  the ports and keep everything inside the layer real.
- An end-to-end test uses a real file, real adapters, and a real Bubble Tea
  program. Keep these tests few and short: they verify wiring rather than the
  behaviour of individual components.

## Placement and naming

- End-to-end tests live in `internal/e2e` with package name `e2e`. All other
  tests live beside the code they cover.
- Keep non-end-to-end tests in the same package as the production code so they
  can exercise intentionally unexported behaviour. Do not export production APIs
  solely to make them testable.
- Put fixtures and helpers in `<subject>_fixtures_test.go`; these files declare
  no `Test` functions.
- A corpus two packages are checked against is the exception: Go cannot share a
  fixture file across packages, and a corpus written out twice lets a document
  added for one package go unchecked by the other. Put it in a package of its
  own, as `internal/application/testdocs` does.
- Name tests as present-tense sentences about behaviour from the subject's point
  of view. A failure should read as a statement of what stopped being true.

## Test doubles

- Use doubles only for ports. Name them `fake<Port>` and keep them beside the
  tests that use them.
- When asserting generated content, use the real renderer, cursor rule, or
  layout. Otherwise the test agrees with a copy of the behaviour to verify.
- Use spies only to verify selection, arguments, or call counts, not to assert
  that generated content is correct.

## Bubble Tea programs

- Assert the final screen from the model returned by `teatest`, not from
  terminal output. Terminal output contains incremental cell updates and is
  platform-dependent. Search it only as a barrier that confirms the initial
  screen was written.
- Use `teatest` only in tests that need to inspect intermediate screens.
- Take intermediate screens from a model that records what it drew, not from
  terminal output. Read them once each and in the order they were drawn: a state
  a scenario waits for is rarely distinguishable from one the program was in
  earlier, so a screen an earlier wait has gone past must not answer later ones.
