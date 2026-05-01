SPEC_URL = https://api.freelo.io/docs/v1/freelo-api.yaml
SPEC     = spec/freelo-api.yaml
GEN      = freeloapi/freeloapi.gen.go

.PHONY: help gen gen-check test vet fmt lint examples tidy

## help: Show this help
help:
	@echo "freelo-go - Development Commands"
	@echo ""
	@sed -n 's/^## //p' Makefile | column -t -s ':'

## gen: Download the spec, apply patches, regenerate the client.
gen:
	@echo "→ download spec from $(SPEC_URL)"
	@curl -fsSL $(SPEC_URL) -o $(SPEC)
	@echo "→ apply Client → BusinessClient rename (avoids HTTP Client name collision)"
	@sed -i.bak 's|^    Client:|    BusinessClient:|; s|#/components/schemas/Client|#/components/schemas/BusinessClient|g' $(SPEC)
	@rm -f $(SPEC).bak
	@echo "→ run oapi-codegen"
	@go generate ./freeloapi/...
	@echo "→ rewrite time.Time → freelotime.Time"
	@go run ./scripts/patchgen
	@echo "→ gofmt"
	@gofmt -w $(GEN)
	@echo "✓ generation complete"

## gen-check: Run gen and fail CI if the working tree is dirty.
gen-check: gen
	@if ! git diff --quiet -- $(GEN) $(SPEC); then \
		echo "ERROR: generated client / spec is out of sync. Run 'make gen' and commit."; \
		git diff --stat -- $(GEN) $(SPEC); \
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
