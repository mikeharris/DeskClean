# Change these variables as necessary.
MAIN_PACKAGE_PATH := .
BUILD_DIR := /tmp/bin/
BINARY_NAME := DeskClean 
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
BUILD := $(shell date +%s)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
VERSION := $(shell git describe --tags --always --abbrev=0 | tr -d '\n')

# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

.PHONY: no-dirty
no-dirty:
	git diff --exit-code

.PHONY: mike
mike:
	@echo ${BUILD_DATE}
	@echo ${BUILD}
	@echo ${GIT_COMMIT}
	@echo ${VERSION}

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy: format code and tidy modfile
.PHONY: tidy
tidy:
	go fmt ./...
	go mod tidy -v

## audit: run quality control checks
.PHONY: audit
audit:
	go mod verify
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest -show verbose ./...
	go test -race -buildvcs -vet=off ./...


# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## test: run all tests
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## build: build the application
.PHONY: build
build:
	go build -o=${BUILD_DIR}${BINARY_NAME} -ldflags "-X main.version=$(VERSION) -X main.build=$(BUILD) -X main.buildDate=$(BUILD_DATE) -X main.commit=$(GIT_COMMIT)" ${MAIN_PACKAGE_PATH}

## run: run the  application
.PHONY: run
run: build
	${BUILD_DIR}${BINARY_NAME}

## package/mac: package the fyne app for Mac
.PHONY: package/mac
package/mac:
	rm -Rf DeskClean.app
	fyne package -os darwin

.PHONY: package/win
package/win:
	#rm DeskClean.exe
	fyne package -os windows

.PHONY: package/linux
package/linux:
	fyne package -os linux

.PHONY: package/mac/install
package/install:
	fyne install

# ==================================================================================== #
# OPERATIONS
# ==================================================================================== #

## push: push changes to the remote Git repository
.PHONY: push
push: tidy audit no-dirty
	git push

## production/deploy: deploy the application to production
.PHONY: production/deploy
production/deploy: confirm tidy audit no-dirty
	GOOS=linux GOARCH=amd64 go build -ldflags='-s' -o=/tmp/bin/linux_amd64/${BINARY_NAME} ${MAIN_PACKAGE_PATH}
	upx -5 /tmp/bin/linux_amd64/${BINARY_NAME}
	# Include additional deployment steps here...
