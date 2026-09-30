.PHONY: help fmt vet test race build verify web-build web-test web-e2e acceptance-v1 estate-acceptance-v1 release

help:
	@echo "make harness-setup Install repository development dependencies"
	@echo "make harness-test Validate queue and run offline launcher regression tests"
	@echo "make harness-validate Run core development and contract checks"
	@echo "make harness-validate-web Run UI unit, build, and browser checks"
	@echo "make fmt     Format Go source"
	@echo "make vet     Run go vet"
	@echo "make test    Run unit and integration tests"
	@echo "make race    Run tests with the race detector"
	@echo "make build   Build bin/aios"
	@echo "make web-build Build the embedded production web assets"
	@echo "make web-test Run web unit tests"
	@echo "make web-e2e  Run browser UI regression tests"
	@echo "make acceptance-v1 ACCEPTANCE_OUTPUT=/private/run Run hermetic V1 core acceptance"
	@echo "make estate-acceptance-v1 ESTATE_CORPUS=/private/corpus.json ACCEPTANCE_OUTPUT=/private/run Run the private real-estate gate"
	@echo "make release VERSION=x.y.z OUTPUT_DIR=/private/releases Build minimal V1 archive"
	@echo "make verify  Run formatting, vet, tests, race tests, and the normal build"

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

build:
	mkdir -p bin
	go build -o bin/aios ./cmd/aios

web-build:
	npm --prefix web run build

web-test:
	npm --prefix web test

web-e2e:
	npm --prefix web run test:e2e

acceptance-v1: build
	@test -n "$(ACCEPTANCE_OUTPUT)" || (echo "ACCEPTANCE_OUTPUT is required" >&2; exit 2)
	python3 scripts/v1_acceptance.py --binary "$(CURDIR)/bin/aios" --output-dir "$(ACCEPTANCE_OUTPUT)"

estate-acceptance-v1: build
	@test -n "$(ESTATE_CORPUS)" -a -n "$(ACCEPTANCE_OUTPUT)" || (echo "ESTATE_CORPUS and ACCEPTANCE_OUTPUT are required" >&2; exit 2)
	python3 scripts/v1_estate_acceptance.py --binary "$(CURDIR)/bin/aios" --corpus "$(ESTATE_CORPUS)" --output-dir "$(ACCEPTANCE_OUTPUT)"

release:
	@test -n "$(VERSION)" -a -n "$(OUTPUT_DIR)" || (echo "VERSION and OUTPUT_DIR are required" >&2; exit 2)
	./scripts/build-v1-release.sh --version "$(VERSION)" --output-dir "$(OUTPUT_DIR)"

verify: fmt vet test race build web-build web-test web-e2e

.PHONY: harness-setup harness-test harness-validate harness-validate-web
harness-setup:
	sh scripts/codex_setup.sh

harness-test:
	python3 scripts/codex_harness.py check
	python3 -m unittest discover -s scripts -p 'test_codex_harness.py'

harness-validate: harness-test
	python3 -m unittest discover -s scripts -p 'test_*.py'
	go vet ./...
	go test ./...
	go test -race ./...
	$(MAKE) build

harness-validate-web: web-test web-build web-e2e
