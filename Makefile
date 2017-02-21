NAME=tpc
VERSION=$(shell cat version)
BUILD_TIME=$(shell date)
BUILD_USER=$(shell whoami)
BUILD_HASH=$(shell git rev-parse HEAD 2>/dev/null || echo "")
ARCH=amd64
OS=linux darwin

LDFLAGS=-ldflags "-X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.BuildUser=$(BUILD_USER)' -X 'main.BuildHash=$(BUILD_HASH)'"

all: clean test build

clean:
	go clean
	@rm -f `which ${NAME}`

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
			env GOOS=$${os} GOARCH=$${arch} go build -v ${LDFLAGS} -o dist/${NAME}-${VERSION}-$${os}-$${arch}; \
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
			tar czf dist/${NAME}-${VERSION}-$${os}-$${arch}.tar.gz -C dist ${NAME}-${VERSION}-$${os}-$${arch} ${NAME}-${VERSION}-$${os}-$${arch}.asc; \
		done; \
	done; \
	find dist/ -type f  ! -name "*.tar.gz" -delete

tag:
	scripts/tag.sh

upload:
	if [ ! -z "\${GITHUB_TOKEN}" ]; then \
		ghr -t "${GITHUB_TOKEN}" -u DripEmail -r ${NAME} -replace ${VERSION} dist/; \
	fi

release: package tag upload

.PHONY: all clean test build distclean dist sign package tag upload release
