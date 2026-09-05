# Conformance entry points

Run `make test` from the repository root after `make setup`.

- `sdk/python/tests/test_basic.py`: shared JSON Schema valid/invalid cases and equivalent active/replay/production ingress fixtures in `protocol/examples/`.
- `internal/core/core_test.go`: default/replace/disable semantics, JSON integer fidelity, input isolation, errors, recovery, directory relocation, bundle integrity and cross-field relationships.
- `internal/server/server_test.go`: CLI-shared execution through HTTP, SSE, export/import, and viewer without Python.

The production fixture represents a completed event, not a production ingestion service.
