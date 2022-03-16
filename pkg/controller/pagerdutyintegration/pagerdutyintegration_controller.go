// Copyright 2020 Red Hat
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

package pagerdutyintegration

import (
	"context"
	"os"
	"time"

	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/pagerduty-operator/config"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "pagerdutyintegration"
)

var fedramp = os.Getenv("FEDRAMP") == "true"
var log = logf.Log.WithName("controller_pagerdutyintegration")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PagerDutyIntegration Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePagerDutyIntegration{
		client:   utils.NewClientWithMetricsOrDie(log, mgr, controllerName),
		scheme:   mgr.GetScheme(),
		pdclient: pd.NewClient,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("pagerdutyintegration-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PagerDutyIntegration
	err = c.Watch(&source.Kind{Type: &pagerdutyv1alpha1.PagerDutyIntegration{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployments, and queue a request for all
	// PagerDutyIntegration CR that selects it.
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: clusterDeploymentToPagerDutyIntegrationsMapper{
				Client: mgr.GetClient(),
			},
		},
	)
	if err != nil {
		return err
	}

	// Watch for changes to SyncSets. If one has any ClusterDeployment owner
	// references, queue a request for all PagerDutyIntegration CR that
	// select those ClusterDeployments.
	err = c.Watch(&source.Kind{Type: &hivev1.SyncSet{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: ownedByClusterDeploymentToPagerDutyIntegrationsMapper{
				Client: mgr.GetClient(),
			},
		},
	)
	if err != nil {
		return err
	}

	// Watch for changes to Secrets. If one has any ClusterDeployment owner
	// references, queue a request for all PagerDutyIntegration CR that
	// select those ClusterDeployments.
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: ownedByClusterDeploymentToPagerDutyIntegrationsMapper{
				Client: mgr.GetClient(),
			},
		},
	)
	if err != nil {
		return err
	}

	// Watch for changes to ConfigMaps. If one has any ClusterDeployment
	// owner references, queue a request for all PagerDutyIntegration CR
	// that select those ClusterDeployments.
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: ownedByClusterDeploymentToPagerDutyIntegrationsMapper{
				Client: mgr.GetClient(),
			},
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcilePagerDutyIntegration implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePagerDutyIntegration{}

// ReconcilePagerDutyIntegration reconciles a PagerDutyIntegration object
type ReconcilePagerDutyIntegration struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	scheme    *runtime.Scheme
	reqLogger logr.Logger
	pdclient  func(APIKey string, controllerName string) pd.Client
}

// Reconcile reads that state of the cluster for a PagerDutyIntegration object and makes changes based on the state read
// and what is in the PagerDutyIntegration.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePagerDutyIntegration) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()

	r.reqLogger = log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	r.reqLogger.Info("Reconciling PagerDutyIntegration")
	if len(os.Getenv("FEDRAMP")) == 0 {
		r.reqLogger.Info("FEDRAMP environment variable unset, defaulting to false")
	} else {
		r.reqLogger.Info("running in FedRAMP environment: %b", fedramp)
	}

	defer func() {
		dur := time.Since(start)
		localmetrics.SetReconcileDuration(controllerName, dur.Seconds())
		r.reqLogger.WithValues("Duration", dur).Info("Reconcile complete")
	}()

	// Fetch the PagerDutyIntegration instance
	pdi := &pagerdutyv1alpha1.PagerDutyIntegration{}
	err := r.client.Get(context.TODO(), request.NamespacedName, pdi)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return r.doNotRequeue()
		}
		// Error reading the object - requeue the request.
		return r.requeueOnErr(err)
	}

	// fetch all CDs so we can inspect if they're dropped out of the matching CD list
	allClusterDeployments, err := r.getAllClusterDeployments()
	if err != nil {
		return reconcile.Result{}, err
	}

	// fetch matching CDs
	matchingClusterDeployments, err := r.getMatchingClusterDeployments(pdi)
	if err != nil {
		return reconcile.Result{}, err
	}

	// the name of the finalizer for the PDI being reconciled
	clusterDeploymentFinalizerName := config.PagerDutyFinalizerPrefix + pdi.Name

	// load PD api key
	pdApiKey, err := utils.LoadSecretData(
		r.client,
		pdi.Spec.PagerdutyApiKeySecretRef.Name,
		pdi.Spec.PagerdutyApiKeySecretRef.Namespace,
		config.PagerDutyAPISecretKey,
	)
	if err != nil {
		r.reqLogger.Error(err, "Failed to load PagerDuty API key from Secret listed in PagerDutyIntegration CR")
		localmetrics.UpdateMetricPagerDutyIntegrationSecretLoaded(0, pdi.Name)
		return r.requeueAfter(10 * time.Minute)
	}
	localmetrics.UpdateMetricPagerDutyIntegrationSecretLoaded(1, pdi.Name)
	pdClient := r.pdclient(pdApiKey, controllerName)

	// check if PDI is being deleted, if so we cleanup all CD w/ matching finalizers
	if pdi.DeletionTimestamp != nil {
		if utils.HasFinalizer(pdi, config.PagerDutyIntegrationFinalizer) {
			// review _all_ CD, cleanup anything w/ this PDI finalizer

			// do the CD cleanup
			for _, clusterdeployment := range allClusterDeployments.Items {
				if utils.HasFinalizer(&clusterdeployment, clusterDeploymentFinalizerName) {
					err = r.handleDelete(pdClient, pdi, &clusterdeployment)
					if err != nil {
						return reconcile.Result{}, err
					}
				}
			}

			localmetrics.DeleteMetricPagerDutyIntegrationSecretLoaded(pdi.Name)

			// do the PDI cleanup
			utils.DeleteFinalizer(pdi, config.PagerDutyIntegrationFinalizer)
			err = r.client.Update(context.TODO(), pdi)
			if err != nil {
				return r.requeueOnErr(err)
			}
		}
		return r.doNotRequeue()
	}

	// add finalizer to PDI if it's not there (if we get here, it's not being deleted)
	if !utils.HasFinalizer(pdi, config.PagerDutyIntegrationFinalizer) {
		utils.AddFinalizer(pdi, config.PagerDutyIntegrationFinalizer)
		err := r.client.Update(context.TODO(), pdi)
		if err != nil {
			return r.requeueOnErr(err)
		}
	}

	// review all CD and see if PD service needs added or removed
	for _, cd := range allClusterDeployments.Items {
		if utils.HasFinalizer(&cd, clusterDeploymentFinalizerName) {
			if cd.DeletionTimestamp != nil {
				// it has a finalizer and is being deleted.  clean up PD things!
				err := r.handleDelete(pdClient, pdi, &cd)
				if err != nil {
					return r.requeueOnErr(err)
				}
			} else {
				// it has a finalizer and is NOT being deleted.
				// check if it should have PD setup or not (did it drop out of the PDI?)
				cdIsMatching := false
				for _, mcd := range matchingClusterDeployments.Items {
					if cd.Namespace == mcd.Namespace && cd.Name == mcd.Name {
						cdIsMatching = true
						break
					}
				}

				if !cdIsMatching {
					// the CD has a finalizer but is NOT matching the PDI. clean it up.
					err := r.handleDelete(pdClient, pdi, &cd)
					if err != nil {
						return r.requeueOnErr(err)
					}
				}
			}
		}
	}

	// and finally, any Matching CD not being deleted
	for _, cd := range matchingClusterDeployments.Items {
		if cd.DeletionTimestamp == nil {
			if err := r.handleCreate(pdClient, pdi, &cd); err != nil {
				return r.requeueOnErr(err)
			}

			if err := r.handleHibernation(pdClient, pdi, &cd); err != nil {
				return r.requeueOnErr(err)
			}

			if err := r.handleLimitedSupport(pdClient, pdi, &cd); err != nil {
				return r.requeueOnErr(err)
			}
		}
	}

	return r.doNotRequeue()
}

func (r *ReconcilePagerDutyIntegration) getAllClusterDeployments() (*hivev1.ClusterDeploymentList, error) {
	allClusterDeployments := &hivev1.ClusterDeploymentList{}
	err := r.client.List(context.TODO(), allClusterDeployments, &client.ListOptions{})
	return allClusterDeployments, err
}
func (r *ReconcilePagerDutyIntegration) getMatchingClusterDeployments(pdi *pagerdutyv1alpha1.PagerDutyIntegration) (*hivev1.ClusterDeploymentList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&pdi.Spec.ClusterDeploymentSelector)
	if err != nil {
		return nil, err
	}

	matchingClusterDeployments := &hivev1.ClusterDeploymentList{}
	listOpts := &client.ListOptions{LabelSelector: selector}
	err = r.client.List(context.TODO(), matchingClusterDeployments, listOpts)
	return matchingClusterDeployments, err
}
func (r *ReconcilePagerDutyIntegration) doNotRequeue() (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (r *ReconcilePagerDutyIntegration) requeueOnErr(err error) (reconcile.Result, error) {
	return reconcile.Result{}, err
}

func (r *ReconcilePagerDutyIntegration) requeueAfter(t time.Duration) (reconcile.Result, error) {
	return reconcile.Result{RequeueAfter: t}, nil
}
