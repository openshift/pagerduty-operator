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
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("localmetrics")

const (
	apiEndpoint = "https://api.pagerduty.com/users"
)

var (
	MetricPagerDutyCreateFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metric_pageduty_create_failure",
		Help: "Metric for the failure of creating a cluster deployment.",
	}, []string{"name", "clusterdeployment_name"})

	MetricPagerDutyDeleteFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metric_pageduty_delete_failure",
		Help: "Metric for the failure of deleting a cluster deployment.",
	}, []string{"name", "clusterdeployment_name"})

	MetricPagerDutyHeartbeat = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "metric_pageduty_heartbeat",
		Help: "Metric for heartbeating of the Pager Duty api.",
	}, []string{"name"})

	MetricsList = []prometheus.Collector{
		MetricPagerDutyCreateFailure,
		MetricPagerDutyDeleteFailure,
		MetricPagerDutyHeartbeat,
	}
)

// UpdateAPIMetrics updates all API endpoint metrics ever 5 minutes
func UpdateAPIMetrics(APIkey string) {
	d := time.Tick(5 * time.Minute)
	for range d {
		UpdateMetricPagerDutyHeartbeatGauge(APIkey)
	}

}

// UpdateMetricPagerDutyCreateFailure updates guage to 1 when creation fails
func UpdateMetricPagerDutyCreateFailure(x int, cd string) {
	MetricPagerDutyCreateFailure.With(prometheus.Labels{
		"name": "pagerduty-operator", "clusterdeployment_name": cd}).Set(
		float64(x))
}

// UpdateMetricPagerDutyDeleteFailure updates guage to 1 when deletion fails
func UpdateMetricPagerDutyDeleteFailure(x int, cd string) {
	MetricPagerDutyDeleteFailure.With(prometheus.Labels{
		"name": "pagerduty-operator", "clusterdeployment_name": cd}).Set(
		float64(x))
}

// UpdateMetricPagerDutyHeartbeatGauge curls the PD API, updates the gauge to 1
// when successful.
func UpdateMetricPagerDutyHeartbeatGauge(APIkey string) {
	metricLogger := log.WithValues("Namespace", "pagerduty-operator")
	metricLogger.Info("Metrics for PD API")

	_, err := http.NewRequest("GET", "api.pagerduty.com", nil)
	if err != nil {
		MetricPagerDutyHeartbeat.With(prometheus.Labels{
			"name": "pagerduty-operator"}).Set(float64(0))
		metricLogger.Error(err, "Failed to get reach api")
	}

	// if there is an api key make an authenticated calld
	if APIkey != "" {
		req, _ := http.NewRequest("GET", apiEndpoint, nil)
		req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Token token="+APIkey)

		resp, err := http.DefaultClient.Do(req)

		if err != nil {
			metricLogger.Error(err, "Failed to reach api when authenticated")
			MetricPagerDutyHeartbeat.With(prometheus.Labels{
				"name": "pagerduty-operator"}).Set(float64(0))
			return
		}
		defer resp.Body.Close()

	}
	MetricPagerDutyHeartbeat.With(prometheus.Labels{
		"name": "pagerduty-operator"}).Set(float64(1))
}
