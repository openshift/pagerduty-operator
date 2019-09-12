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

package syncset

import (
	"context"

	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"

	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_syncset")

const syncsetPostfix string = "-pd-sync"

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new SyncSet Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	newRec, err := newReconciler(mgr)
	if err != nil {
		return err
	}

	return add(mgr, newRec)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	//return &ReconcileSyncSet{client: mgr.GetClient(), scheme: mgr.GetScheme()}

	tempClient, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return nil, err
	}

	// get PD API key from secret
	pdAPIKey, err := utils.LoadSecretData(tempClient, config.PagerDutyAPISecretName, config.OperatorNamespace, config.PagerDutyAPISecretKey)

	return &ReconcileSyncSet{
		client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		pdclient: pd.NewClient(pdAPIKey),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("syncset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource SyncSet
	err = c.Watch(&source.Kind{Type: &hivev1.SyncSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSyncSet{}

// ReconcileSyncSet reconciles a SyncSet object
type ReconcileSyncSet struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	scheme    *runtime.Scheme
	reqLogger logr.Logger
	pdclient  pd.Client
}

func (r *ReconcileSyncSet) checkClusterDeployment(request reconcile.Request) (bool, error) {
	clusterdeployment := &hivev1.ClusterDeployment{}
	cdName := request.Name[0 : len(request.Name)-8]

	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: cdName}, clusterdeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			r.reqLogger.Info("No matching cluster deployment found, ignoring")
			return false, nil
		}
		// Error finding the cluster deployment, requeue
		return false, err
	}

	if clusterdeployment.DeletionTimestamp != nil {
		return false, nil
	}

	return true, nil
}

// Reconcile reads that state of the cluster for a SyncSet object and makes changes based on the state read
// and what is in the SyncSet.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileSyncSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.reqLogger = log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	r.reqLogger.Info("Reconciling SyncSet")

	// Wasn't a pagerduty
	if len(request.Name) < len(syncsetPostfix) {
		return reconcile.Result{}, nil
	}
	if request.Name[len(request.Name)-len(syncsetPostfix):len(request.Name)] != syncsetPostfix {
		return reconcile.Result{}, nil
	}

	// Fetch the SyncSet instance
	instance := &hivev1.SyncSet{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			isCDCreated, checkerr := r.checkClusterDeployment(request)
			if checkerr != nil {
				return reconcile.Result{}, checkerr
			}

			if isCDCreated {
				return r.recreateSyncSet(request)
			}
			// ClusterDeployment was deleted, do nothing
			return reconcile.Result{}, nil

		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
