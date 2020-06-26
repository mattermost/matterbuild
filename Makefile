GO ?= $(shell command -v go 2> /dev/null)

PACKAGES=$(shell go list ./...)

## Checks the code style, tests and builds.
.PHONY: all
all: check-style test build

## Cleans workspace
.PHONY: clean
clean:
	rm -rf dist/ out/

## Runs govet and gofmt against all packages.
.PHONY: check-style
check-style: gofmt govet
	@echo Checking for style guide compliance

## Runs gofmt against all packages.
.PHONY: gofmt
gofmt:
	@echo Running GOFMT

	@for package in $(PACKAGES) ; do \
		echo "Checking "$$package; \
		files=$$(go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}} {{end}}' $$package); \
		if [ "$$files" ]; then \
			gofmt_output=$$(gofmt -d -s $$files 2>&1); \
			if [ "$$gofmt_output" ]; then \
				echo "$$gofmt_output"; \
				echo "gofmt failure"; \
				exit 1; \
			fi; \
		fi; \
	done
	@echo "gofmt success"; \

## Runs govet against all packages.
.PHONY: govet
govet:
	@echo Running govet
	env GO111MODULE=off $(GO) get golang.org/x/tools/go/analysis/passes/shadow/cmd/shadow
	$(GO) vet $(PACKAGES) || exit 1
	$(GO) vet -vettool=$(GOPATH)/bin/shadow $(PACKAGES) || exit 1
	@echo Govet success

## Runs the matterbuild server.
.PHONY: run
run: NOTILT ?=
run:
ifeq (, $(shell which tilt))
	go run matterbuild.go
else
ifneq (, $(NOTILT))
	go run matterbuild.go
else
	tilt up --web-mode prod
endif
endif

## Runs test against all packages.
.PHONY: test
test:
	$(GO) test -v -race ./...

## Builds matterbuild.
.PHONY: build
build: clean
	@echo Building
	$(GO) build -o dist/matterbuild

# Docker variables
DEFAULT_TAG  ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD 2>/dev/null)
DOCKER_IMAGE ?= mattermost/matterbuild
DOCKER_TAG   ?= $(shell echo "$(DEFAULT_TAG)" | tr -d 'v')

## Build Docker image
.PHONY: docker
docker:
	docker build --pull --tag $(DOCKER_IMAGE):$(DOCKER_TAG) --file Dockerfile .

## Push Docker image
.PHONY: push
push:
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

## Generate mocks.
.PHONY: mocks
mocks:
	go install github.com/golang/mock/mockgen
	mockgen -package mocks -destination server/mocks/mock_github_repo.go github.com/mattermost/matterbuild/server GithubRepositoriesService
	mockgen -package mocks -destination server/mocks/mock_github_search.go github.com/mattermost/matterbuild/server GithubSearchService
	mockgen -package mocks -destination server/mocks/mock_github_git.go github.com/mattermost/matterbuild/server GithubGitService

#####################
## Release targets ##
#####################
PATTERN =

# if the last release was alpha, beta or rc, 'release' target has to used with current
# cycle release. For example if latest tag is v0.8.0-rc.2 and v0.8.0 GA needs to get
# released the following should be executed: "make release version=0.8.0"
## Prepare release
.PHONY: release
release: VERSION ?= $(shell git describe --tags 2>/dev/null | sed 's/^v//' | awk -F'[ .]' '{print $(PATTERN)}')
release:
	@ ./scripts/release.sh "$(VERSION)" "1"

## Prepare Patch release
.PHONY: patch
patch: PATTERN = '\$$1\".\"\$$2\".\"\$$3+1'
patch: release

## Prepare Minor release
.PHONY: minor
minor: PATTERN = '\$$1\".\"\$$2+1\".0\"'
minor: release

## Prepare Major release
.PHONY: major
major: PATTERN = '\$$1+1\".0.0\"'
major: release

# Help documentation à la https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@cat Makefile | grep -v '\.PHONY' |  grep -v '\help:' | grep -B1 -E '^[a-zA-Z_.-]+:.*' | sed -e "s/:.*//" | sed -e "s/^## //" |  grep -v '\-\-' | uniq | sed '1!G;h;$$!d' | awk 'NR%2{printf "\033[36m%-30s\033[0m",$$0;next;}1' | sort
