.PHONY: all
all: checks unit-tests unit-tests-race

.PHONY: checks
checks: check-deps
	@test -z $(shell gofmt -l -s $(shell go list -f '{{.Dir}}' ./... | grep -v mpc) | tee /dev/stderr) || (echo "Fix formatting issues"; exit 1)
	@go vet -all $(shell go list -f '{{.Dir}}' ./... | grep -v mpc)
	find . -name '*.go' | xargs addlicense -check || (echo "Missing license headers"; exit 1)

.PHONY: unit-tests
unit-tests:
	@go test -timeout 480s -cover $(shell go list ./...)

.PHONY: unit-tests-race
unit-tests-race:
	@export GORACE=history_size=7; go test -timeout 960s -race -cover $(shell go list ./...)

.PHONY: perf
perf:
	@go test -benchmem -bench=Benchmark_Sequential.* -run=^$$ -v
	@go test -benchmem -bench=Benchmark_Parallel.* -run=^$$ -cpu=1,2,4,8,16,32,64 -v

.PHONY: check-deps
check-deps:
	@go install github.com/google/addlicense@latest

.PHONY: lint
# run various linters
lint:
	@echo "Running Go Linters..."
	golangci-lint run --color=always --timeout=4m

.PHONY: lint-auto-fix
# run linters with auto-fix
lint-auto-fix:
	@echo "Running Go Linters with auto-fix..."
	golangci-lint run --color=always --timeout=4m --fix

.PHONY: install-linter-tool
# install golangci-lint
install-linter-tool:
	@echo "Installing golangci Linter"
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(HOME)/go/bin v2.12.2

.PHONY: fmt
fmt: ## Run gofmt on the entire project
	@echo "Running gofmt..."
	@gofmt -l -s -w .
