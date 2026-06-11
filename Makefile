SHELL        := $(shell which bash)

# Tooling is installed into BIN_DIR via `go run <tool>@<version>` so versions are
# pinned without network-piped install scripts.
LINTER       := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
TESTRUNNER   := go run gotest.tools/gotestsum@v1.13.0
VULNCHECKER  := go run golang.org/x/vuln/cmd/govulncheck@v1.3.0
COVER_FLOOR  := 80
FUZZTIME     ?= 10s
# Lazily expanded: only `fmt` needs it, and CI containers shouldn't have to
# care whether git trusts the workspace.
ROOT_DIR     = $(shell git rev-parse --show-toplevel)
NO_COLOR     :=\033[0m
ATTN_COLOR   :=\033[33;01m

## EOF define block

.PHONY: all
all: fmt test race lint cover vuln

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
	@CGO_ENABLED=0 go test -count=1 -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | awk -v floor=$(COVER_FLOOR) \
		'/^total:/ { sub(/%/, "", $$3); printf "total coverage: %s%% (floor: %s%%)\n", $$3, floor; exit ($$3 + 0 < floor) ? 1 : 0 }'

.PHONY: vet
vet:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@go vet ./...

.PHONY: lint
lint:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@CGO_ENABLED=0 $(LINTER) run ./...

# The vulnerability database is fetched live on every run; the pin above only
# fixes the scanner binary itself.
.PHONY: vuln
vuln:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@$(VULNCHECKER) ./...

# Go only fuzzes one target per invocation, hence one line per target. The
# seed corpora also run as plain tests under `make test`.
.PHONY: fuzz
fuzz:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@CGO_ENABLED=0 go test -run='^$$' -fuzz=FuzzPathSeg -fuzztime=$(FUZZTIME) .
	@CGO_ENABLED=0 go test -run='^$$' -fuzz=FuzzDebugBody -fuzztime=$(FUZZTIME) .

.PHONY: clean
clean:
	@echo -e "$(ATTN_COLOR)==> $@ $(NO_COLOR)"
	@go clean
	@rm -f coverage.*
