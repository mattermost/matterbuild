GO ?= $(shell command -v go 2> /dev/null)
DEP ?= $(shell command -v dep 2> /dev/null)

PACKAGES=$(shell go list ./...)

## Checks the code style, tests and builds.
.PHONY: all
all: check-style test build

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
run:
	go run matterbuild.go

## Runs test against all packages.
.PHONY: test
test:
	$(GO) test -v -race ./...

## Builds matterbuild.
.PHONY: build
build:
	@echo Building

	rm -rf dist/
	mkdir -p dist/matterbuild
	$(GO) build
	mv matterbuild dist/matterbuild/
	cp config.json dist/matterbuild/

## Generate mocks.
.PHONY: mocks
mocks:
	go install github.com/golang/mock/mockgen
	mockgen -package mocks -destination server/mocks/mock_github_repo.go github.com/mattermost/matterbuild/server GithubRepositoriesService
	mockgen -package mocks -destination server/mocks/mock_github_search.go github.com/mattermost/matterbuild/server GithubSearchService
	mockgen -package mocks -destination server/mocks/mock_github_git.go github.com/mattermost/matterbuild/server GithubGitService

## Packages matterbuild.
.PHONY: package
package: build
	tar -C dist -czf dist/matterbuild.tar.gz matterbuild

# Help documentation à la https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@cat Makefile | grep -v '\.PHONY' |  grep -v '\help:' | grep -B1 -E '^[a-zA-Z_.-]+:.*' | sed -e "s/:.*//" | sed -e "s/^## //" |  grep -v '\-\-' | sed '1!G;h;$$!d' | awk 'NR%2{printf "\033[36m%-30s\033[0m",$$0;next;}1' | sort
