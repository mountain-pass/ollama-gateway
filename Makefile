.PHONY: build run test

build:
	CGO_ENABLED=0 go build -o ollama-gateway .

run: build
	OLLAMA_BASE_URL=http://localhost:11434 \
	API_TOKENS=test-token \
	./ollama-gateway

test:
	go test -v -race ./...
