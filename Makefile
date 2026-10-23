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
	python3 -m unittest discover -s scripts -p 'test_release_tools.py' -v
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

.PHONY: release acceptance-install acceptance-install-linux
RELEASE_PYTHON ?= python3.11
RELEASE_OS ?= $(shell go env GOOS)
RELEASE_ARCH ?= $(shell go env GOARCH)
release: build
	python3 scripts/package_release.py --python "$(RELEASE_PYTHON)" --os "$(RELEASE_OS)" --arch "$(RELEASE_ARCH)"
acceptance-install: release
	python3 scripts/installation_acceptance.py --archive "artifacts/t13/releases/seven-agent-eval-$(RELEASE_OS)-$(RELEASE_ARCH).tar.gz" --python "$(RELEASE_PYTHON)" --output "artifacts/t13/$(RELEASE_OS)-$(RELEASE_ARCH)"
acceptance-install-linux: build
	python3 scripts/package_release.py --python "$(RELEASE_PYTHON)" --os linux --arch arm64
	python3 scripts/installation_linux.py

.PHONY: acceptance-clean
acceptance-clean:
	python3 scripts/clean_acceptance.py --python "$(RELEASE_PYTHON)"
