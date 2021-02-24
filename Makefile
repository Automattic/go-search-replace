BINARY = go-search-replace
BUILDDIR = ./build

all: vet fmt lint test build

build: clean
	which gox > /dev/null || go get -u github.com/mitchellh/gox
	gox -os="linux" -os="darwin" -os="windows" -arch="amd64" -arch="386" -arch="arm64" -output="${BUILDDIR}/${BINARY}_{{.OS}}_{{.Arch}}"
	gzip build/*

vet:
	go vet ./...

fmt:
	gofmt -s -l . | grep -v vendor | tee /dev/stderr

lint:
	golint ./... | grep -v vendor | tee /dev/stderr

test:
	go test -v ./...
	go test -bench .

clean:
	rm -rf ${BUILDDIR}

.PHONY: all clean vet fmt lint test build
