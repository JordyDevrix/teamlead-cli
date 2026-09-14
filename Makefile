.PHONY: all build test clean install lint

BINARY_NAME=teamlead-cli

all: test build

build:
	go build -ldflags="-s -w" -o $(BINARY_NAME) .
	ln -sf $(BINARY_NAME) teamlead

test:
	go test -v -count=1 ./...

install:
	go install .

clean:
	rm -f $(BINARY_NAME) teamlead
	go clean -testcache
