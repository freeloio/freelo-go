SPEC_URL = https://api.freelo.io/docs/v1/freelo-api.yaml
SPEC     = spec/freelo-api.yaml
META     = spec/.freelo-api.meta.json
GEN      = freeloapi/freeloapi.gen.go

# Extra flags for the spec fetcher, e.g. `make gen FETCH_FLAGS=-force` to
# bypass the ETag/If-Modified-Since cache for one run.
FETCH_FLAGS ?=

.PHONY: help gen gen-force gen-check test vet fmt lint examples tidy

## help: Show this help
help:
	@echo "freelo-go - Development Commands"
	@echo ""
	@sed -n 's/^## //p' Makefile | column -t -s ':'

## gen: Download the spec, apply patches, regenerate the client.
gen:
	@echo "→ fetch spec from $(SPEC_URL) (conditional GET)"
	@go run ./scripts/fetchspec -url $(SPEC_URL) -out $(SPEC) -meta $(META) $(FETCH_FLAGS)
	@echo "→ apply Client → BusinessClient rename (avoids HTTP Client name collision)"
	@sed -i.bak 's|^    Client:|    BusinessClient:|; s|#/components/schemas/Client|#/components/schemas/BusinessClient|g' $(SPEC)
	@rm -f $(SPEC).bak
	@echo "→ flatten scalar oneOf parameter schemas (unions break oapi-codegen)"
	@go run ./scripts/patchspec
	@echo "→ record patched spec fingerprint"
	@go run ./scripts/fetchspec -seal -out $(SPEC) -meta $(META)
	@echo "→ run oapi-codegen"
	@go generate ./freeloapi/...
	@echo "→ rewrite time.Time → freelotime.Time"
	@go run ./scripts/patchgen
	@echo "→ gofmt"
	@gofmt -w $(GEN)
	@echo "✓ generation complete"

## gen-force: Run gen, ignoring the cached ETag/Last-Modified validators.
gen-force:
	@$(MAKE) gen FETCH_FLAGS=-force

## gen-check: Run gen and fail CI if the working tree is dirty.
gen-check: gen
	@if ! git diff --quiet -- $(GEN) $(SPEC) $(META); then \
		echo "ERROR: generated client / spec is out of sync. Run 'make gen' and commit."; \
		git diff --stat -- $(GEN) $(SPEC) $(META); \
		exit 1; \
	fi

## test: Run unit tests.
test:
	go test -race ./...

## vet: Run go vet on every package.
vet:
	go vet ./...

## fmt: Verify gofmt is clean. Fails if any file would be rewritten.
fmt:
	@diff=$$(gofmt -l .); \
	if [ -n "$$diff" ]; then \
		echo "gofmt is not clean. Run 'gofmt -w .' on:"; \
		echo "$$diff"; \
		exit 1; \
	fi

## lint: fmt + vet (no third-party linter — add golangci-lint later if needed).
lint: fmt vet

## examples: Build every example package — catches API drift in tutorials.
examples:
	@for d in examples/*/; do \
		echo "→ build $$d"; \
		(cd "$$d" && go build ./...) || exit 1; \
	done

## tidy: go mod tidy.
tidy:
	go mod tidy
