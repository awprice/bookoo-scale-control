.PHONY: build test vet check demo

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

check: vet test

demo:
	go build -o bookoo ./cmd/bookoo/
	PROMPT_COMMAND="" PS1="> " vhs vhs/demo.tape
