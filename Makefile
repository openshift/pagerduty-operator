FIPS_ENABLED=true
include boilerplate/generated-includes.mk

# Temporary until boilerplated
include functions.mk
CATALOG_REGISTRY_ORGANIZATION?=app-sre

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update

# Extend Makefile after here

.PHONY: skopeo-push
skopeo-push:
	skopeo copy \
		--dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
		"docker-daemon:${OPERATOR_IMAGE_URI_LATEST}" \
		"docker://${OPERATOR_IMAGE_URI_LATEST}"
	skopeo copy \
		--dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
		"docker-daemon:${OPERATOR_IMAGE_URI}" \
		"docker://${OPERATOR_IMAGE_URI}"

.PHONY: build-catalog-image
build-catalog-image:
	$(call create_push_catalog_image,staging,service/saas-pagerduty-operator-bundle,$$APP_SRE_BOT_PUSH_TOKEN,false,service/app-interface,data/services/osd-operators/cicd/saas/saas-$(OPERATOR_NAME).yaml,hack/generate-operator-bundle.py,$(CATALOG_REGISTRY_ORGANIZATION),$(OPERATOR_NAME))
	$(call create_push_catalog_image,production,service/saas-pagerduty-operator-bundle,$$APP_SRE_BOT_PUSH_TOKEN,true,service/app-interface,data/services/osd-operators/cicd/saas/saas-$(OPERATOR_NAME).yaml,hack/generate-operator-bundle.py,$(CATALOG_REGISTRY_ORGANIZATION),$(OPERATOR_NAME))


.PHONY: test-unit
test-unit:
	@echo "running unit tests..."
	go test -cover -v -race ./cmd/... ./pkg/...