.PHONY: build test vet check demo

build:
	go build -o bookoo ./cmd/bookoo/

test:
	go test ./...

vet:
	go vet ./...

check: vet test

demo: build
	PROMPT_COMMAND="" PS1="> " vhs vhs/demo.tape
