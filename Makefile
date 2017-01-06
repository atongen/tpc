VERSION=$(shell cat version)
BUILD_TIME=$(shell date)
BUILD_USER=$(shell whoami)
BUILD_HASH=$(shell git rev-parse HEAD 2>/dev/null || echo "")
ARCH=amd64
OS=linux

LDFLAGS=-ldflags "-X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.BuildUser=$(BUILD_USER)' -X 'main.BuildHash=$(BUILD_HASH)'"

all: clean test build

clean:
	go clean

test:
	go test -cover

build: test
	go install ${LDFLAGS}

distclean:
	@mkdir -p dist
	rm -rf dist/*

dist: test distclean
	for arch in ${ARCH}; do \
		for os in ${OS}; do \
			env GOOS=$${os} GOARCH=$${arch} go build -v ${LDFLAGS} -o dist/tpc-${VERSION}-$${os}-$${arch}; \
		done; \
	done

sign: dist
	$(eval key := $(shell git config --get user.signingkey))
	for file in dist/*; do \
		gpg2 --armor --local-user ${key} --detach-sign $${file}; \
	done

package: sign
	for arch in ${ARCH}; do \
		for os in ${OS}; do \
			tar czf dist/tpc-${VERSION}-$${os}-$${arch}.tar.gz -C dist tpc-${VERSION}-$${os}-$${arch} tpc-${VERSION}-$${os}-$${arch}.asc; \
		done; \
	done

tag:
	scripts/tag.sh

release: package tag

.PHONY: all clean test build distclean dist sign package tag release
