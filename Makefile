FIPS_ENABLED=true

# Prow CI image ships Go 1.25; go.mod is 1.26 (k8s 0.36). Auto-select toolchain.
export GOTOOLCHAIN=go1.26.4+auto

include boilerplate/generated-includes.mk

.PHONY: go-check
go-check:
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2
	${GOENV} PATH="$$(go env GOPATH)/bin:$$PATH" GOLANGCI_LINT_CACHE=${GOLANGCI_LINT_CACHE} golangci-lint run -c ${CONVENTION_DIR}/golangci.yml $(if $(LINT_NEW_FROM_REV),--new-from-rev=$(LINT_NEW_FROM_REV)) ./...

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update
