package utils

import (
	"context"

	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasFinalizer returns true if the given object has the given finalizer
func HasFinalizer(object metav1.Object, finalizer string) bool {
	finalizers := sets.NewString(object.GetFinalizers()...)
	return finalizers.Has(finalizer)
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

// DeleteConfigMap deletes a ConfigMap
func DeleteConfigMap(name string, namespace string, client client.Client, reqLogger logr.Logger) error {
	configmap := &v1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, configmap)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return nil
		}
		// Error finding the object, requeue
		return err
	}

	reqLogger.Info("Deleting ConfigMap", "Namespace", namespace, "Name", name)
	err = client.Delete(context.TODO(), configmap)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return nil
		}
		// Error finding the object, requeue
		return err
	}

	return nil
}

// DeleteSyncSet deletes a SyncSet
func DeleteSyncSet(name string, namespace string, client client.Client, reqLogger logr.Logger) error {
	syncset := &hivev1.SyncSet{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, syncset)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return nil
		}
		// Error finding the syncset, requeue
		return err
	}

	// Only delete the syncset, this is just cleanup of the synced secret.
	// The ClusterDeployment controller manages deletion of the pagerduty serivce.
	reqLogger.Info("Deleting SyncSet", "Namespace", namespace, "Name", name)
	err = client.Delete(context.TODO(), syncset)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return nil
		}
		// Error finding the syncset, requeue
		return err
	}

	return nil
}

// DeleteSecret deletes a Secret
func DeleteSecret(name string, namespace string, client client.Client, reqLogger logr.Logger) error {
	secret := &v1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, secret)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return nil
		}
		// Error finding the secret, requeue
		return err
	}

	reqLogger.Info("Deleting Secret", "Namespace", namespace, "Name", name)
	err = client.Delete(context.TODO(), secret)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return nil
		}
		// Error finding the secret, requeue
		return err
	}

	return nil
}
