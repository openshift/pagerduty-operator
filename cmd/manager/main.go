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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/operator-custom-metrics/pkg/metrics"
	operatorconfig "github.com/openshift/pagerduty-operator/config"
	"github.com/openshift/pagerduty-operator/pkg/apis"
	"github.com/openshift/pagerduty-operator/pkg/controller"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	routev1 "github.com/openshift/api/route/v1"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

// Change below variables to serve metrics on different host or port.
var (
	// metricsPort the port on which metrics is hosted, don't pick one that's already used
	metricsPort = "8081"
	metricsPath = "/metrics"
)
var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.Logger())

	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "pagerduty-operator-lock")
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// get the namespaces that are being watched
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	options := manager.Options{
		Namespace: namespace,
		// disable the controller-runtime metrics
		MetricsBindAddress: "0",
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	// Note that this is not intended to be used for excluding namespaces, this is better done via a Predicate
	// Also note that you may face performance issues when using this with a high number of namespaces.
	// More Info: https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/cache#MultiNamespacedCacheBuilder
	if strings.Contains(namespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(namespace, ","))
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, options)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err := hivev1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "error registering prometheus monitoring objects")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	metricsServer := metrics.NewBuilder(operatorconfig.OperatorNamespace, operatorconfig.OperatorName).
		WithPort(metricsPort).
		WithPath(metricsPath).
		WithCollectors(localmetrics.MetricsList).
		WithRoute().
		GetConfig()

	// Configure metrics if it errors log the error but continue
	if err := metrics.ConfigureMetrics(context.TODO(), *metricsServer); err != nil {
		log.Error(err, "Failed to configure Metrics")
		os.Exit(1)
	}

	// Add runnable custom metrics
	err = mgr.Add(manager.RunnableFunc(func(s <-chan struct{}) error {
		client := mgr.GetClient()
		pdAPISecret := &corev1.Secret{}
		err = client.Get(context.TODO(), types.NamespacedName{Namespace: operatorconfig.OperatorNamespace, Name: operatorconfig.PagerDutyAPISecretName}, pdAPISecret)
		if err != nil {
			log.Error(err, "Failed to get secret")
			return err
		}
		var APIKey = string(pdAPISecret.Data[operatorconfig.PagerDutyAPISecretKey])
		timer := prometheus.NewTimer(localmetrics.MetricPagerDutyHeartbeat)
		localmetrics.UpdateAPIMetrics(APIKey, timer)

		<-s
		return nil
	}))
	if err != nil {
		log.Error(err, "unable add a runnable to the manager")
		os.Exit(1)
	}

	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}
