# Test data conventions

Hermetic Go tests build temporary Evaluation Packages with the `fixture` and `put` helpers in `internal/core/core_test.go`; no credentials or external services are needed.
Shared language-neutral protocol fixtures live in `protocol/examples/`.
Runnable user-facing Packages live in `examples/history/` and `examples/http/`.
Generated acceptance bundles and browser artifacts are ignored by Git.
