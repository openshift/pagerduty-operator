module github.com/openshift/pagerduty-operator

go 1.13

require (
	cloud.google.com/go v0.57.0 // indirect
	github.com/PagerDuty/go-pagerduty v1.5.0
	github.com/go-logr/logr v0.2.1
	github.com/go-openapi/spec v0.19.4
	github.com/golang/mock v1.4.4
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/openshift/api v3.9.1-0.20191111211345-a27ff30ebf09+incompatible
	github.com/openshift/hive/apis v0.0.0-20211217134024-32a93e12e129
	github.com/openshift/operator-custom-metrics v0.3.1-0.20200901174648-463079905232
	github.com/operator-framework/operator-sdk v0.17.2
	github.com/prometheus/client_golang v1.7.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	golang.org/x/tools v0.0.0-20200616195046-dc31b401abb5 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	k8s.io/api v0.20.0
	k8s.io/apiextensions-apiserver v0.19.0 // indirect
	k8s.io/apimachinery v0.20.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20201113171705-d219536bb9fd
	sigs.k8s.io/controller-runtime v0.6.2
)

// from installer
replace (
	github.com/Azure/go-autorest => github.com/tombuildsstuff/go-autorest v14.0.1-0.20200416184303-d4e299a3c04a+incompatible
	github.com/Azure/go-autorest/autorest => github.com/tombuildsstuff/go-autorest/autorest v0.10.1-0.20200416184303-d4e299a3c04a
	github.com/Azure/go-autorest/autorest/azure/auth => github.com/tombuildsstuff/go-autorest/autorest/azure/auth v0.4.3-0.20200416184303-d4e299a3c04a
	github.com/metal3-io/baremetal-operator => github.com/openshift/baremetal-operator v0.0.0-20200715132148-0f91f62a41fe
	github.com/metal3-io/cluster-api-provider-baremetal => github.com/openshift/cluster-api-provider-baremetal v0.0.0-20190821174549-a2a477909c1d
	github.com/terraform-providers/terraform-provider-aws => github.com/openshift/terraform-provider-aws v1.60.1-0.20200630224953-76d1fb4e5699
	github.com/terraform-providers/terraform-provider-azurerm => github.com/openshift/terraform-provider-azurerm v1.40.1-0.20200707062554-97ea089cc12a
	github.com/terraform-providers/terraform-provider-ignition/v2 => github.com/community-terraform-providers/terraform-provider-ignition/v2 v2.1.0
	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20200506073438-9d49428ff837
	sigs.k8s.io/cluster-api-provider-azure => github.com/openshift/cluster-api-provider-azure v0.1.0-alpha.3.0.20200120114645-8a9592f1f87b
	sigs.k8s.io/cluster-api-provider-openstack => github.com/openshift/cluster-api-provider-openstack v0.0.0-20200526112135-319a35b2e38e
)

// needed because otherwise v12.0.0 is picked up as a more recent version
replace k8s.io/client-go => k8s.io/client-go v0.19.0

replace k8s.io/api => k8s.io/api v0.19.0

// needed because otherwise installer fetches a library-go version that requires bitbucket.com/ww/goautoneg which is dead
// Tagged version fetches github.com/munnerz/goautoneg instead
replace github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20200918101923-1e4c94603efe

// temporary hack fix for https://github.com/kubernetes/kubernetes/issues/95300
replace k8s.io/apiserver => github.com/staebler/apiserver v0.19.1-0.20201005174924-a3ef0d1e45df

// replace zapr with version matching logr
replace github.com/go-logr/zapr => github.com/go-logr/zapr v0.2.0
