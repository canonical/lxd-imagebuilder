VERSION=$(shell grep "var Version" shared/version/version.go | cut -d'"' -f2)
ARCHIVE=lxd-imagebuilder-$(VERSION).tar
GO111MODULE=on
SPHINXENV=.sphinx/venv/bin/activate
GO_MIN=1.23.3

.PHONY: default
default:
	go env -w GOCACHE=$(shell go env GOCACHE)
	$(shell go env | grep -v GOENV | sed "s/'//g" > $(shell go env GOENV))
	gofmt -s -w .
	go install -v ./...
	@echo "lxd-imagebuilder and simplestream-maintainer built successfully"

.PHONY: update-gomod
update-gomod:
	go get -t -v -u ./...
	go mod tidy -go=$(GO_MIN)
	@echo "Dependencies updated"

.PHONY: check
check: default
	sudo GOENV=$(shell go env GOENV) go test -v ./...

.PHONY: dist
dist:
	# Cleanup
	rm -Rf $(ARCHIVE).gz

	# Create build dir
	$(eval TMP := $(shell mktemp -d))
	git archive --prefix=lxd-imagebuilder-$(VERSION)/ HEAD | tar -x -C $(TMP)
	mkdir -p $(TMP)/_dist/src/github.com/canonical
	ln -s ../../../../lxd-imagebuilder-$(VERSION) $(TMP)/_dist/src/github.com/canonical/lxd-imagebuilder

	# Download dependencies
	cd $(TMP)/lxd-imagebuilder-$(VERSION) && go mod vendor

	# Assemble tarball
	tar --exclude-vcs -C $(TMP) -zcf $(ARCHIVE).gz lxd-imagebuilder-$(VERSION)/

	# Cleanup
	rm -Rf $(TMP)

.PHONY: doc-setup
doc-setup:
	make -C doc clean
	make -C doc install

.PHONY: doc
doc: doc-setup doc-incremental

.PHONY: doc-incremental
doc-incremental:
	make -C doc html

.PHONY: doc-serve
doc-serve:
	make -C doc serve

.PHONY: doc-spellcheck
doc-spellcheck:
	make -C doc spelling

.PHONY: doc-linkcheck
doc-linkcheck:
	make -C doc linkcheck

.PHONY: doc-woke
doc-woke:
	make -C doc woke

.PHONY: static-analysis
static-analysis:
ifeq ($(shell command -v golangci-lint 2> /dev/null),)
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.57.1
endif
	golangci-lint run --timeout 5m
	run-parts $(shell run-parts -V 2> /dev/null 1> /dev/null && echo -n "--exit-on-error --regex '.sh'") test/lint
