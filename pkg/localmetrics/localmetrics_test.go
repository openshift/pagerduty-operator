package localmetrics

import (
	neturl "net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathParse(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		path     string
		expected string
	}{
		{
			name:     "core non-namespaced kind",
			host:     "172.30.0.1:443",
			path:     "/api/v1/pods",
			expected: "core/v1/pods",
		},
		{
			name:     "core non-namespaced named resource",
			host:     "172.30.0.1:443",
			path:     "/api/v1/nodes/nodename",
			expected: "core/v1/nodes/{NAME}",
		},
		{
			name:     "core namespaced named resource",
			host:     "172.30.0.1:443",
			path:     "/api/v1/namespaces/pagerduty-operator/configmaps/foo-bar-baz",
			expected: "core/v1/namespaces/{NAMESPACE}/configmaps/{NAME}",
		},
		{
			name:     "core namespaced named resource with sub-resource",
			host:     "172.30.0.1:443",
			path:     "/api/v1/namespaces/pagerduty-operator/secret/foo-bar-baz/status",
			expected: "core/v1/namespaces/{NAMESPACE}/secret/{NAME}/status",
		},
		{
			name:     "extension non-namespaced kind",
			host:     "172.30.0.1:443",
			path:     "/apis/batch/v1/jobs",
			expected: "batch/v1/jobs",
		},
		{
			name:     "extension namespaced kind",
			host:     "172.30.0.1:443",
			path:     "/apis/batch/v1/namespaces/pagerduty-operator/jobs",
			expected: "batch/v1/namespaces/{NAMESPACE}/jobs",
		},
		{
			name:     "extension namespaced named resource",
			host:     "172.30.0.1:443",
			path:     "/apis/batch/v1/namespaces/pagerduty-operator/jobs/foo-bar-baz",
			expected: "batch/v1/namespaces/{NAMESPACE}/jobs/{NAME}",
		},
		{
			name:     "extension namespaced named resource with sub-resource",
			host:     "172.30.0.1:443",
			path:     "/apis/pd.managed.openshift.io/v1alpha1/namespaces/pagerduty-operator/accountpool/foo-bar-baz/status",
			expected: "pd.managed.openshift.io/v1alpha1/namespaces/{NAMESPACE}/accountpool/{NAME}/status",
		},
		{
			name:     "core root (discovery)",
			host:     "172.30.0.1:443",
			path:     "/api",
			expected: "core",
		},
		{
			name:     "core version (discovery)",
			host:     "172.30.0.1:443",
			path:     "/api/v1",
			expected: "core/v1",
		},
		{
			name:     "extension discovery",
			host:     "172.30.0.1:443",
			path:     "/apis/pagerduty.managed.openshift.io/v1",
			expected: "pagerduty.managed.openshift.io/v1",
		},
		{
			name:     "unknown root",
			host:     "172.30.0.1:443",
			path:     "/weird/path/to/resource",
			expected: "{OTHER}",
		},
		{
			name:     "empty to make Split fail",
			host:     "172.30.0.1:443",
			path:     "",
			expected: "{OTHER}",
		},
		{
			name:     "access to escalation policies",
			host:     "pagerduty.com",
			path:     "/escalation_policies/PPP12345XXX",
			expected: "pagerduty.com/escalation_policies/{UID}",
		},
		{
			name:     "retrieve service",
			host:     "pagerduty.com",
			path:     "/services/PPP12345XXX",
			expected: "pagerduty.com/services/{UID}",
		},
		{
			name:     "retrieve integration",
			host:     "pagerduty.com",
			path:     "/services/PPP12345XXX/integrations/PPP12345XXX",
			expected: "pagerduty.com/services/{UID}/integrations/{UID}",
		},
		{
			name:     "ignore further subresources",
			host:     "pagerduty.com",
			path:     "/services/PPP12345XXX/integrations/PPP12345XXX/subresource/PPP12345XXX",
			expected: "pagerduty.com/services/{UID}/integrations/{UID}",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := resourceFrom(&neturl.URL{Host: test.host, Path: test.path})
			assert.Equal(t, test.expected, result)
		})
	}

}
