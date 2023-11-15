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

.PHONY: check-deps
check-deps:
	@go install github.com/google/addlicense@latest

.PHONY: idemixgen
idemixgen:
	@go install ./tools/idemixgen

.PHONY: binaries
binaries:
	mkdir -p bin/amd64
	GOOS=linux GOARCH=amd64 go build -o bin/amd64/idemixgen tools/idemixgen/main.go

	mkdir -p bin/arm64
	GOOS=darwin GOARCH=arm64 go build -o bin/arm64/idemixgen tools/idemixgen/main.go

.PHONY: genprotos
genprotos:
	go install github.com/bufbuild/buf/cmd/buf@v1.1.1
	buf generate --template buf.gen.yaml
