package utils

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// HasFinalizer returns true if the given object has the given finalizer
func HasFinalizer(object metav1.Object, finalizer string) bool {
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds a finalizer to the given object
func AddFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Insert(finalizer)
	object.SetFinalizers(finalizers.List())
}

// DeleteFinalizer removes a finalizer from the given object
func DeleteFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Delete(finalizer)
	object.SetFinalizers(finalizers.List())
}

// CheckClusterDeployment returns true if the ClusterDeployment is watched by this operator
func CheckClusterDeployment(request reconcile.Request, client client.Client, reqLogger logr.Logger) (bool, *hivev1.ClusterDeployment, error) {

	// remove SyncSetPostfix from name to lookup the ClusterDeployment
	cdName := strings.Replace(request.NamespacedName.Name, config.SyncSetPostfix, "", 1)
	cdNamespace := request.NamespacedName.Namespace

	clusterDeployment := &hivev1.ClusterDeployment{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: cdName, Namespace: cdNamespace}, clusterDeployment)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("No matching cluster deployment found, ignoring")
			return false, clusterDeployment, nil
		}
		// Error finding the cluster deployment, requeue
		return false, clusterDeployment, err
	}

	if clusterDeployment.DeletionTimestamp != nil {
		return false, clusterDeployment, nil
	}

	if val, ok := clusterDeployment.GetLabels()[config.ClusterDeploymentManagedLabel]; ok {
		if val != "true" {
			reqLogger.Info("Is not a managed cluster")
			return false, clusterDeployment, nil
		}
	} else {
		// Managed tag is not present which implies it is not a managed cluster
		reqLogger.Info("Is not a managed cluster")
		return false, clusterDeployment, nil
	}

	// Return if alerts are disabled on the cluster
	if _, ok := clusterDeployment.GetLabels()[config.ClusterDeploymentNoalertsLabel]; ok {
		reqLogger.Info("Managed cluster with Alerts disabled", "Namespace", request.Namespace, "Name", request.Name)
		return false, clusterDeployment, nil
	}

	// made it this far so it's both managed and has alerts enabled
	return true, clusterDeployment, nil
}
