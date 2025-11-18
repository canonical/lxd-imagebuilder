VERSION=$(shell grep "var Version" shared/version/version.go | cut -d'"' -f2)
ARCHIVE=lxd-imagebuilder-$(VERSION).tar
GO111MODULE=on
GOTOOLCHAIN=local
export GOTOOLCHAIN
SPHINXENV=.sphinx/venv/bin/activate
GOMIN=1.25.4

.PHONY: default
default:
	go env -w GOCACHE=$(shell go env GOCACHE)
	$(shell go env | grep -v GOENV | sed "s/'//g" > $(shell go env GOENV))
	gofmt -s -w .
	go install -v ./...
	@echo "lxd-imagebuilder and simplestream-maintainer built successfully"

.PHONY: update-gomin
update-gomin:
ifndef NEW_GOMIN
	@echo "Usage: make update-gomin NEW_GOMIN=1.x.y"
	@echo "Current Go minimum version: $(GOMIN)"
	exit 1
endif
ifeq "$(GOMIN)" "$(NEW_GOMIN)"
	@echo "Error: NEW_GOMIN ($(NEW_GOMIN)) is the same as current GOMIN ($(GOMIN))"
	exit 1
endif
	@echo "Updating Go minimum version from $(GOMIN) to $(NEW_GOMIN)"

	@# Update GOMIN in Makefile
	sed -i 's/^GOMIN=[0-9.]\+/GOMIN=$(NEW_GOMIN)/' Makefile

	@# Update GOMIN in go.mod
	sed -i 's/^go [0-9.]\+$$/go $(NEW_GOMIN)/' go.mod

	@echo "Go minimum version updated to $(NEW_GOMIN)"
	if [ -t 0 ]; then \
		read -rp "Would you like to commit Go version changes (Y/n)? " answer; \
		if [ "$${answer:-y}" = "y" ] || [ "$${answer:-y}" = "Y" ]; then \
			git commit -S -sm "go: Update Go minimum version to $(NEW_GOMIN)" -- Makefile go.mod; \
		fi; \
	fi

.PHONY: update-gomod
update-gomod:
	go get -t -v -u ./...

	# Enforce minimum go version
	$(MAKE) check-gomin

	# Use the bundled toolchain that meets the minimum go version
	go get toolchain@none

	@echo "Dependencies updated"

.PHONY: check
check: check-gomin default
	$(shell go env | grep -v GOENV | sed "s/'//g" > $(shell go env GOENV))
	go test -v ./...

.PHONY: check-gomin
check-gomin:
	go mod tidy -go=$(GOMIN)

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
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$HOME/go/bin
endif
	golangci-lint run --timeout 5m
	run-parts $(shell run-parts -V 2> /dev/null 1> /dev/null && echo -n "--exit-on-error --regex '.sh'") test/lint
