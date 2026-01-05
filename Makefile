.PHONY: fmt lint test build e2e clean

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...

test:
	go test -v ./...

build:
	go build -o bin/hcloud-k8s ./cmd/hcloud-k8s

e2e:
	go test -v -timeout=1h -tags=e2e ./tests/e2e/...

clean:
	rm -rf bin/
