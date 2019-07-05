
GOX=$(shell which gox > /dev/null 2>&1 ; echo $$? )
PACKR=$(shell which packr > /dev/null 2>&1 ; echo $$? )

.PHONY: build
build: static-assets
	@echo "--> Build"
ifeq (1,${GOX})
	@echo "--> Error: Install gox to build minimal. See https://github.com/mitchellh/gox"
	exit 1
endif
	@sh -c ./scripts/build.sh

.PHONY: static-assets
static-assets:
	@echo "--> Generating static assets for the json chains"

ifeq (1,${PACKR})
	@echo "--> Error: Install packr to generate static assets. See https://github.com/gobuffalo/packr"
	exit 1
endif
	@packr -i ./chain

.PHONY: clean
clean:
	@echo "--> Cleaning build artifacts"
ifeq (1,${PACKR})
	@echo "--> Error: Install packr to generate static assets. See https://github.com/gobuffalo/packr"
	exit 1
endif
	@packr clean
