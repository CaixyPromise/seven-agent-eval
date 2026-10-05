.PHONY: setup build test demo acceptance-live acceptance-rag acceptance-seven acceptance-compare acceptance-typescript acceptance-resilience browser-fixture
setup:
	uv sync --locked --project sdk/python --python 3.11
	pnpm --dir web install --frozen-lockfile
	pnpm --dir sdk/typescript install --frozen-lockfile
	pnpm --dir sdk/typescript build
build:
	pnpm --dir sdk/typescript build
	pnpm --dir web build
	go build -o bin/seven-eval ./cmd/seven-eval
test:
	pnpm --dir sdk/typescript build
	pnpm --dir sdk/typescript test
	go test -race ./...
	pnpm --dir web test
	uv run --project sdk/python python -m unittest discover -s sdk/python/tests -v
	pnpm --dir web build
demo: build
	./bin/seven-eval --package examples/history run smoke
acceptance-live: build
	uv run --project sdk/python python scripts/live_acceptance.py
browser-fixture: build
	uv run --project sdk/python python scripts/browser_fixture.py prepare
acceptance-rag: build
	uv run --project sdk/python python scripts/phase3_acceptance.py
acceptance-seven: build
	uv run --project sdk/python python scripts/seven_acceptance.py
acceptance-compare: build
	uv run --project sdk/python python scripts/comparison_acceptance.py
acceptance-typescript: build
	uv run --project sdk/python python scripts/typescript_acceptance.py

acceptance-resilience: build
	uv run --project sdk/python python scripts/resilience_acceptance.py

.PHONY: acceptance-large
acceptance-large: build
	uv run --project sdk/python python scripts/large_acceptance.py

.PHONY: acceptance-online-capacity
acceptance-online-capacity: build
	uv run --project sdk/python python scripts/online_capacity_acceptance.py
