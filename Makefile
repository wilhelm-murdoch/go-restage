SHELL        := $(shell which bash)

# Tooling is installed into BIN_DIR via `go run <tool>@<version>` so versions are
# pinned without network-piped install scripts.
LINTER       := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
TESTRUNNER   := go run gotest.tools/gotestsum@v1.13.0
ROOT_DIR     := $(shell git rev-parse --show-toplevel)
NO_COLOR     :=\033[0m
ATTN_COLOR   :=\033[33;01m

## EOF define block

.PHONY: all
all: fmt test race lint cover

.PHONY: deps
deps:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@go mod download

.PHONY: tidy
tidy:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@go mod tidy

.PHONY: fmt
fmt:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@gofmt -w $(ROOT_DIR)

.PHONY: test
test:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@CGO_ENABLED=0 $(TESTRUNNER) --format short-verbose -- -count=1 ./...

.PHONY: race
race:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@CGO_ENABLED=1 go test -race -count=1 ./...

.PHONY: cover
cover:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@CGO_ENABLED=0 go test -count=1 -cover ./...

.PHONY: vet
vet:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@go vet ./...

.PHONY: lint
lint:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@CGO_ENABLED=0 $(LINTER) run ./...

.PHONY: clean
clean:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@go clean
	@rm -f coverage.*
