
.PHONY: static-assets
static-assets:
	@echo "--> Generating static assets for the json chains"
	@packr -i ./chain

.PHONY: clean
clean:
	@echo "--> Cleaning build artifacts"
	@packr clean
