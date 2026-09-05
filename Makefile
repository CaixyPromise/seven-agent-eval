.PHONY: setup build test demo
setup:
	uv sync --locked --project sdk/python --python 3.11
	pnpm --dir web install --frozen-lockfile
build:
	pnpm --dir web build
	go build -o bin/seven-eval ./cmd/seven-eval
test:
	go test -race ./...
	pnpm --dir web test
	uv run --project sdk/python python -m unittest discover -s sdk/python/tests -v
	pnpm --dir web build
demo: build
	./bin/seven-eval --package examples/history run smoke
