// Copyright 2019 RedHat
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package localmetrics

import (
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("localmetrics")

const (
	apiEndpoint  = "https://api.pagerduty.com/users"
	operatorName = "pagerduty-operator"
)

var (
	MetricPagerDutyCreateFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "pagerduty_create_failure",
		Help:        "Metric for the failure of creating a cluster deployment.",
		ConstLabels: prometheus.Labels{"name": "pagerduty-operator"},
	}, []string{"clusterdeployment_name", "pagerdutyintegration_name"})

	MetricPagerDutyDeleteFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "pagerduty_delete_failure",
		Help:        "Metric for the failure of deleting a cluster deployment.",
		ConstLabels: prometheus.Labels{"name": "pagerduty-operator"},
	}, []string{"clusterdeployment_name", "pagerdutyintegration_name"})

	MetricPagerDutyHeartbeat = prometheus.NewSummary(prometheus.SummaryOpts{
		Name:        "pagerduty_heartbeat",
		Help:        "Metric for heartbeating of the Pager Duty api.",
		ConstLabels: prometheus.Labels{"name": "pagerduty-operator"},
	})

	ReconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "pagerduty_operator_reconcile_duration_seconds",
		Help:        "Distribution of the number of seconds a Reconcile takes, broken down by controller",
		ConstLabels: prometheus.Labels{"name": "pagerduty-operator"},
		Buckets:     []float64{0.001, 0.01, 0.1, 1, 5, 10, 20},
	}, []string{"controller"})

	// apiCallDuration times API requests. Histogram also gives us a _count metric for free.
	ApiCallDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "pagerduty_operator_api_request_duration_seconds",
		Help:        "Distribution of the number of seconds an API request takes",
		ConstLabels: prometheus.Labels{"name": operatorName},
		// We really don't care about quantiles, but omitting Buckets results in defaults.
		// This minimizes the number of unused data points we store.
		Buckets: []float64{1},
	}, []string{"controller", "method", "resource", "status"})

	MetricPagerDutyIntegrationSecretLoaded = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "pagerdutyintegration_secret_loaded",
		Help:        "Metric to track the ability to load the PagerDuty API key from the Secret specified in the PagerDutyIntegration",
		ConstLabels: prometheus.Labels{"name": "pagerduty-operator"},
	}, []string{"pagerdutyintegration_name"})

	MetricsList = []prometheus.Collector{
		MetricPagerDutyCreateFailure,
		MetricPagerDutyDeleteFailure,
		MetricPagerDutyHeartbeat,
		ApiCallDuration,
		ReconcileDuration,
		MetricPagerDutyIntegrationSecretLoaded,
	}
)

// UpdateAPIMetrics updates all API endpoint metrics every 5 minutes
func UpdateAPIMetrics(APIKey string, timer *prometheus.Timer) {
	d := time.Tick(5 * time.Minute)
	for range d {
		UpdateMetricPagerDutyHeartbeat(APIKey, timer)
	}

}

// UpdateMetricPagerDutyIntegrationSecretLoaded updates gauge to 1
// when the PagerDuty API key can be loaded from the Secret specified
// in the PagerDutyIntegration, or to 0 if it fails
func UpdateMetricPagerDutyIntegrationSecretLoaded(x int, pdiName string) {
	MetricPagerDutyIntegrationSecretLoaded.With(
		prometheus.Labels{"pagerdutyintegration_name": pdiName},
	).Set(float64(x))
}

// DeleteMetricPagerDutyIntegrationSecretLoaded deletes the metric for
// the PagerDutyIntegration name provided. This should be called when
// e.g. the PagerDutyIntegration is being deleted, so that there are
// no irrelevant metrics (which alerts could be firing on).
func DeleteMetricPagerDutyIntegrationSecretLoaded(pdiName string) bool {
	return MetricPagerDutyIntegrationSecretLoaded.Delete(
		prometheus.Labels{"pagerdutyintegration_name": pdiName},
	)
}

// UpdateMetricPagerDutyCreateFailure updates gauge to 1 when creation fails
func UpdateMetricPagerDutyCreateFailure(x int, cd string, pdiName string) {
	MetricPagerDutyCreateFailure.With(prometheus.Labels{
		"clusterdeployment_name":    cd,
		"pagerdutyintegration_name": pdiName,
	}).Set(float64(x))
}

// UpdateMetricPagerDutyDeleteFailure updates gauge to 1 when deletion fails
func UpdateMetricPagerDutyDeleteFailure(x int, cd string, pdiName string) {
	MetricPagerDutyDeleteFailure.With(prometheus.Labels{
		"clusterdeployment_name":    cd,
		"pagerdutyintegration_name": pdiName,
	}).Set(float64(x))
}

// SetReconcileDuration Push the duration of the reconcile iteration
func SetReconcileDuration(controller string, duration float64) {
	ReconcileDuration.WithLabelValues(controller).Observe(duration)
}

// UpdateMetricPagerDutyHeartbeat curls the PD API, updates the gauge to 1
// when successful.
func UpdateMetricPagerDutyHeartbeat(APIKey string, timer *prometheus.Timer) {
	metricLogger := log.WithValues("Namespace", "pagerduty-operator")
	metricLogger.Info("Metrics for PD API")

	// if there is an api key make an authenticated called
	if APIKey != "" {
		req, _ := http.NewRequest("GET", apiEndpoint, nil)
		req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Token token=%s", APIKey))
		resp, err := http.DefaultClient.Do(req)

		if err != nil {
			metricLogger.Error(err, "Failed to reach api when authenticated")
			MetricPagerDutyHeartbeat.Observe(
				float64(timer.ObserveDuration().Seconds()))

			return
		}
		defer resp.Body.Close()

	}
	MetricPagerDutyHeartbeat.Observe(float64(0))
}

// AddAPICall observes metrics for a call to an external API
// - param controller: The name of the controller making the API call
// - param req: The HTTP Request structure
// - param resp: The HTTP Response structure
// - param duration: The number of seconds the call took.
func AddAPICall(controller string, req *http.Request, resp *http.Response, duration float64) {
	ApiCallDuration.With(prometheus.Labels{
		"controller": controller,
		"method":     req.Method,
		"resource":   resourceFrom(req.URL),
		"status":     resp.Status,
	}).Observe(duration)
}

// resourceFrom normalizes an API request URL, including removing individual namespace and
// resource names, to yield a string of the form:
//     $group/$version/$kind[/{NAME}[/...]]
// or
//     $group/$version/namespaces/{NAMESPACE}/$kind[/{NAME}[/...]]
// ...where $foo is variable, {FOO} is actually {FOO}, and [foo] is optional.
// This is so we can use it as a dimension for the ApiCallDuration metric, without ending up
// with separate labels for each {namespace x name}.
func resourceFrom(url *neturl.URL) (resource string) {
	defer func() {
		// If we can't parse, return a general bucket. This includes paths that don't start with
		// /api or /apis.
		if r := recover(); r != nil {
			// TODO(efried): Should we be logging these? I guess if we start to see a lot of them...
			resource = "{OTHER}"
		}
	}()

	tokens := strings.Split(url.Path[1:], "/")

	// First normalize to $group/$version/...
	switch tokens[0] {
	case "api":
		// Core resources: /api/$version/...
		// => core/$version/...
		tokens[0] = "core"
	case "apis":
		// Extensions: /apis/$group/$version/...
		// => $group/$version/...
		tokens = tokens[1:]
	default:
		// Something else. Punt.
		panic(1)
	}

	// Single resource, non-namespaced (including a namespace itself): $group/$version/$kind/$name
	if len(tokens) == 4 {
		// Factor out the resource name
		tokens[3] = "{NAME}"
	}

	// Kind or single resource, namespaced: $group/$version/namespaces/$nsname/$kind[/$name[/...]]
	if len(tokens) > 4 && tokens[2] == "namespaces" {
		// Factor out the namespace name
		tokens[3] = "{NAMESPACE}"

		// Single resource, namespaced: $group/$version/namespaces/$nsname/$kind/$name[/...]
		if len(tokens) > 5 {
			// Factor out the resource name
			tokens[5] = "{NAME}"
		}
	}

	resource = strings.Join(tokens, "/")

	return
}
