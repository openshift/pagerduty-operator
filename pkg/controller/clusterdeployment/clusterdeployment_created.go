package clusterdeployment

import (
	"context"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileClusterDeployment) handleCreate(request reconcile.Request, instance *hivev1.ClusterDeployment) (reconcile.Result, error) {
	r.reqLogger.Info("Creating syncset")

	newSS, err := pd.GenerateSyncSet(r.client, request.Namespace, instance.Name)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.client.Create(context.TODO(), newSS); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
