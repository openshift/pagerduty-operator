/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pagerdutyintegration

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
)

const controllerName = "pagerdutyintegration"

var log = logf.Log.WithName("controller_pagerdutyintegration")

// pdiReconcileErrors implements the builtin error interface
type pdiReconcileErrors []error

// Error causes pdiReconcileErrors to convert the error to a string like normal unless its length is more than one.
// Then it will print the first error and report the remaining number of errors.
func (p pdiReconcileErrors) Error() string {
	switch len(p) {
	case 0:
		return ""
	case 1:
		return p[0].Error()
	default:
		return fmt.Sprintf("%s - %d other errors", p[0].Error(), len(p)-1)
	}
}

// PagerDutyIntegrationReconciler reconciles a PagerDutyIntegration object
type PagerDutyIntegrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	reqLogger logr.Logger
	pdclient  func(APIKey string, controllerName string) pd.Client
}

//+kubebuilder:rbac:groups=pagerduty.pagerduty.openshift.io,resources=pagerdutyintegrations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=pagerduty.pagerduty.openshift.io,resources=pagerdutyintegrations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=pagerduty.pagerduty.openshift.io,resources=pagerdutyintegrations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *PagerDutyIntegrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()

	if r.pdclient == nil {
		r.pdclient = pd.NewClient
	}
	r.reqLogger = log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	r.reqLogger.Info("Reconciling PagerDutyIntegration")

	defer func() {
		dur := time.Since(start)
		localmetrics.SetReconcileDuration(controllerName, dur.Seconds())
		r.reqLogger.WithValues("Duration", dur).Info("Reconcile complete")
	}()

	// Fetch the PagerDutyIntegration instance
	pdi := &pagerdutyv1alpha1.PagerDutyIntegration{}
	err := r.Get(context.TODO(), req.NamespacedName, pdi)
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
		return r.requeueOnErr(err)
	}

	// Fetch ClusterDeployments matching the PDI's ClusterDeployment label selector
	matchingClusterDeployments, err := r.getMatchingClusterDeployments(pdi)
	if err != nil {
		return r.requeueOnErr(err)
	}

	// the name of the finalizer for the PDI being reconciled
	clusterDeploymentFinalizerName := config.PagerDutyFinalizerPrefix + pdi.Name

	// load PD api key
	pdApiKey, err := utils.LoadSecretData(
		r.Client,
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

	// If the PDI is being deleted, clean up all ClusterDeployments with matching finalizers
	if pdi.DeletionTimestamp != nil {
		if utils.HasFinalizer(pdi, config.PagerDutyIntegrationFinalizer) {
			for _, clusterdeployment := range allClusterDeployments.Items {
				clusterdeployment := clusterdeployment
				if utils.HasFinalizer(&clusterdeployment, clusterDeploymentFinalizerName) {
					err = r.handleDelete(pdClient, pdi, &clusterdeployment)
					if err != nil {
						return r.requeueOnErr(err)
					}
				}
			}

			localmetrics.DeleteMetricPagerDutyIntegrationSecretLoaded(pdi.Name)

			// Once all ClusterDeployments have been cleaned up, delete the PDI finalizer
			utils.DeleteFinalizer(pdi, config.PagerDutyIntegrationFinalizer)
			err = r.Update(context.TODO(), pdi)
			if err != nil {
				return r.requeueOnErr(err)
			}
		}
		return r.doNotRequeue()
	}

	// Ensure the PDI has a finalizer to protect it from deletion
	if !utils.HasFinalizer(pdi, config.PagerDutyIntegrationFinalizer) {
		utils.AddFinalizer(pdi, config.PagerDutyIntegrationFinalizer)
		err := r.Update(context.TODO(), pdi)
		if err != nil {
			return r.requeueOnErr(err)
		}
	}

	var reconcileErrors pdiReconcileErrors
	// Process all ClusterDeployments with the PDI finalizer for PD service deletion
	for _, cd := range allClusterDeployments.Items {
		cd := cd
		if utils.HasFinalizer(&cd, clusterDeploymentFinalizerName) {
			if cd.DeletionTimestamp != nil {
				// The ClusterDeployment is being deleted, so delete the PD service
				err := r.handleDelete(pdClient, pdi, &cd)
				if err != nil {
					reconcileErrors = append(reconcileErrors, err)
				}
			} else {
				// The ClusterDeployment is NOT being deleted, is it one of our matched ClusterDeployments?
				cdIsMatching := false
				for _, mcd := range matchingClusterDeployments.Items {
					if cd.Namespace == mcd.Namespace && cd.Name == mcd.Name {
						cdIsMatching = true
						break
					}
				}

				// It's not a matched ClusterDeployment, delete the PagerDuty service because it shouldn't exist
				if !cdIsMatching {
					r.reqLogger.Info(fmt.Sprintf("cleaning up %s as it has a finalizer but no matching label", cd.Name))
					err := r.handleDelete(pdClient, pdi, &cd)
					if err != nil {
						reconcileErrors = append(reconcileErrors, err)
					}
				}
			}
		}
	}

	// and finally, any Matching CD not being deleted
	for _, cd := range matchingClusterDeployments.Items {
		cd := cd
		if cd.DeletionTimestamp == nil {
			if err := r.handleCreate(pdClient, pdi, &cd); err != nil {
				reconcileErrors = append(reconcileErrors, err)
			}

			// Do nothing if the orchestration is not enabled and leave it as default for now
			if pdi.Spec.ServiceOrchestration.Enabled {
				if err := r.handleServiceOrchestration(pdClient, pdi, &cd); err != nil {
					reconcileErrors = append(reconcileErrors, err)
				}
			}

			if err := r.handleHibernation(pdClient, pdi, &cd); err != nil {
				reconcileErrors = append(reconcileErrors, err)
			}

			if err := r.handleLimitedSupport(pdClient, pdi, &cd); err != nil {
				reconcileErrors = append(reconcileErrors, err)
			}
		}
	}

	if len(reconcileErrors) > 0 {
		return r.requeueOnErr(reconcileErrors)
	}

	return r.doNotRequeue()
}

func (r *PagerDutyIntegrationReconciler) getAllClusterDeployments() (*hivev1.ClusterDeploymentList, error) {
	allClusterDeployments := &hivev1.ClusterDeploymentList{}
	err := r.List(context.TODO(), allClusterDeployments, &client.ListOptions{})
	return allClusterDeployments, err
}

func (r *PagerDutyIntegrationReconciler) getMatchingClusterDeployments(pdi *pagerdutyv1alpha1.PagerDutyIntegration) (*hivev1.ClusterDeploymentList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&pdi.Spec.ClusterDeploymentSelector)
	if err != nil {
		return nil, err
	}

	matchingClusterDeployments := &hivev1.ClusterDeploymentList{}
	listOpts := &client.ListOptions{LabelSelector: selector}
	err = r.List(context.TODO(), matchingClusterDeployments, listOpts)
	return matchingClusterDeployments, err
}

func (r *PagerDutyIntegrationReconciler) doNotRequeue() (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (r *PagerDutyIntegrationReconciler) requeueOnErr(err error) (reconcile.Result, error) {
	return reconcile.Result{}, err
}

func (r *PagerDutyIntegrationReconciler) requeueAfter(t time.Duration) (reconcile.Result, error) {
	return reconcile.Result{RequeueAfter: t}, nil
}

// SetupWithManager sets up the controller with the Manager.
// Custom event handlers are utilized here such that when a ClusterDeployment event is created, only associated
// PagerDutyIntegration CRs are reconciled. Likewise, when events for SyncSets, ConfigMaps, or Secrets are created,
// if they're owned by a ClusterDeployment, then associated PagerDutyIntegration CRs are reconciled.
func (r *PagerDutyIntegrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pagerdutyv1alpha1.PagerDutyIntegration{}).
		Watches(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &enqueueRequestForClusterDeployment{
			Client: mgr.GetClient(),
		}).
		Watches(&source.Kind{Type: &hivev1.SyncSet{}}, &enqueueRequestForClusterDeploymentOwner{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, &enqueueRequestForClusterDeploymentOwner{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).
		Watches(&source.Kind{Type: &corev1.Secret{}}, &enqueueRequestForClusterDeploymentOwner{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, &enqueueRequestForConfigMap{
			Client: mgr.GetClient(),
		}).
		Complete(r)
}
