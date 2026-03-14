.PHONY: lint test test-unit test-e2e update-golden build run clean

GOTESTSUM := go run gotest.tools/gotestsum@latest

lint:
	go fmt ./...
	go vet ./...

test-unit:
	$(GOTESTSUM) --format testdox -- -coverprofile=coverage.out ./...

test-e2e:
	$(GOTESTSUM) --format testdox -- -tags=integration,e2e ./...

test: test-unit test-e2e

update-golden:
	go test ./cmd/... -update

coverage: test-unit
	go tool cover -func=coverage.out

coverage-html: coverage
	go tool cover -html=coverage.out

build:
	go build -o bin/llm-proxy main.go

run: build
	./bin/llm-proxy run server

clean:
	rm -rf bin/
	rm -f coverage.out
