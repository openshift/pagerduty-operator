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

package clusterdeployment

import (
	"context"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	metrics "github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileClusterDeployment) handleDelete(request reconcile.Request, instance *hivev1.ClusterDeployment) (reconcile.Result, error) {
	if instance == nil {
		// nothing to do, bail early
		return reconcile.Result{}, nil
	}

	if !utils.HasFinalizer(instance, config.OperatorFinalizer) {
		return reconcile.Result{}, nil
	}

	ClusterID := instance.Spec.ClusterName

	pdData := &pd.Data{
		ClusterID:  instance.Spec.ClusterName,
		BaseDomain: instance.Spec.BaseDomain,
	}
	err := pdData.ParsePDConfig(r.client)
	deletePDService := true

	if err != nil {
		if !errors.IsNotFound(err) {
			// some error other than not found, requeue
			return reconcile.Result{}, err
		}
		/*
			The PD config was not found.

			If the error is a missing PD Config we must not fail or requeue.
			If we are deleting (we're in handleDelete) and we cannot find the PD config
			it will never be created.  We cannot recover so just skip the PD service
			deletion.
		*/
		deletePDService = false
	}

	if deletePDService {
		err = pdData.ParseClusterConfig(r.client, request.Namespace, request.Name)

		if err != nil {
			if !errors.IsNotFound(err) {
				// some error other than not found, requeue
				return reconcile.Result{}, err
			}
			/*
				Something was not found if we are here.

				The missing object will never be created as we're in the handleDelete function.
				Skip service deletion in this case and continue with deletion.
			*/
			deletePDService = false
		}
	}

	if deletePDService {
		// we have everything necessary to attempt deletion of the PD service
		err = r.pdclient.DeleteService(pdData)
		if err != nil {
			r.reqLogger.Error(err, "Failed cleaning up pagerduty.")
		} else {
			// NOTE not deleting the configmap if we didn't delete the service with the assumption that the config can be used later for cleanup
			// find the PD configmap and delete it
			cmName := request.Name + config.ConfigMapPostfix
			r.reqLogger.Info("Deleting PD ConfigMap", "Namespace", request.Namespace, "Name", cmName)
			err = utils.DeleteConfigMap(cmName, request.Namespace, r.client, r.reqLogger)

			if err != nil {
				r.reqLogger.Error(err, "Error deleting ConfigMap", "Namespace", request.Namespace, "Name", cmName)
			}
		}
	}
	// find the pd secret and delete id
	r.reqLogger.Info("Deleting PD secret", "Namespace", request.Namespace, "Name", config.PagerDutySecretName)
	err = utils.DeleteSecret(config.PagerDutySecretName, request.Namespace, r.client, r.reqLogger)
	if err != nil {
		r.reqLogger.Error(err, "Error deleting Secret", "Namespace", request.Namespace, "Name", config.PagerDutySecretName)
	}

	// find the PD syncset and delete it
	ssName := request.Name + config.SyncSetPostfix
	r.reqLogger.Info("Deleting PD SyncSet", "Namespace", request.Namespace, "Name", ssName)
	err = utils.DeleteSyncSet(ssName, request.Namespace, r.client, r.reqLogger)

	if err != nil {
		r.reqLogger.Error(err, "Error deleting SyncSet", "Namespace", request.Namespace, "Name", ssName)
	}

	if utils.HasFinalizer(instance, config.OperatorFinalizer) {
		r.reqLogger.Info("Deleting PD finalizer from ClusterDeployment", "Namespace", request.Namespace, "Name", request.Name)
		utils.DeleteFinalizer(instance, config.OperatorFinalizer)
		err = r.client.Update(context.TODO(), instance)
		if err != nil {
			metrics.UpdateMetricPagerDutyDeleteFailure(1, ClusterID)
			return reconcile.Result{}, err
		}
	}
	metrics.UpdateMetricPagerDutyDeleteFailure(0, ClusterID)

	return reconcile.Result{}, nil
}
