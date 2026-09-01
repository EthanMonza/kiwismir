BINARY := kiwismir
PKG     := .

.PHONY: all build run tidy test fmt vet clean docker

all: build

## build: compile the bot into ./bin/kiwismir
build:
	@mkdir -p bin
	go build -trimpath -ldflags "-s -w" -o bin/$(BINARY) $(PKG)

## run: build and run using your .env
run:
	go run $(PKG)

## tidy: sync go.mod/go.sum
tidy:
	go mod tidy

## test: run unit tests
test:
	go test ./...

## fmt: gofmt the whole tree
fmt:
	gofmt -s -w .

## vet: run go vet
vet:
	go vet ./...

## clean: remove build artifacts
clean:
	rm -rf bin

## docker: build the container image
docker:
	docker build -t kiwismir:latest .
