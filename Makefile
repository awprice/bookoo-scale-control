.PHONY: build test vet check

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

check: vet test
