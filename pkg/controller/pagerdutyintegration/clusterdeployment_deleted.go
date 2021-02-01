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

package pagerdutyintegration

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/pagerduty-operator/config"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	metrics "github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcilePagerDutyIntegration) handleDelete(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
	if cd == nil {
		// nothing to do, bail early
		return nil
	}

	var (
		// secretName is the name of the Secret deployed to the target
		// cluster, and also the name of the SyncSet that causes it to
		// be deployed.
		secretName string = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.SecretSuffix)

		// configMapName is the name of the ConfigMap containing the
		// SERVICE_ID and INTEGRATION_ID
		configMapName string = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)

		// There can be more than one PagerDutyIntegration that causes
		// creation of resources for a ClusterDeployment, and each one
		// will need a finalizer here. We add a suffix of the CR
		// name to distinguish them.
		finalizer string = config.PagerDutyFinalizerPrefix + pdi.Name
	)

	if !utils.HasFinalizer(cd, finalizer) {
		return nil
	}

	ClusterID := cd.Spec.ClusterName

	deletePDService := true

	pdAPISecret := &corev1.Secret{}
	err := r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      pdi.Spec.PagerdutyApiKeySecretRef.Name,
			Namespace: pdi.Spec.PagerdutyApiKeySecretRef.Namespace,
		},
		pdAPISecret,
	)
	if err != nil {
		if !errors.IsNotFound(err) {
			// some error other than not found, requeue
			return err
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

	apiKey, err := pd.GetSecretKey(pdAPISecret.Data, config.PagerDutyAPISecretKey)
	if err != nil {
		return err
	}

	pdData := &pd.Data{
		ClusterID:          cd.Spec.ClusterName,
		BaseDomain:         cd.Spec.BaseDomain,
		EscalationPolicyID: pdi.Spec.EscalationPolicy,
		AutoResolveTimeout: pdi.Spec.ResolveTimeout,
		AcknowledgeTimeOut: pdi.Spec.AcknowledgeTimeout,
		ServicePrefix:      pdi.Spec.ServicePrefix,
		APIKey:             apiKey,
	}

	if deletePDService {
		err = pdData.ParseClusterConfig(r.client, cd.Namespace, configMapName)

		if err != nil {
			if !errors.IsNotFound(err) {
				// some error other than not found, requeue
				return err
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
		err = pdclient.DeleteService(pdData)
		if err != nil {
			r.reqLogger.Error(err, "Failed cleaning up pagerduty.")
		} else {
			// NOTE: not deleting the configmap if we didn't delete
			// the service with the assumption that the config can
			// be used later for cleanup find the PD configmap and
			// delete it
			r.reqLogger.Info("Deleting PD ConfigMap", "Namespace", cd.Namespace, "Name", configMapName)
			err = utils.DeleteConfigMap(configMapName, cd.Namespace, r.client, r.reqLogger)

			if err != nil {
				r.reqLogger.Error(err, "Error deleting ConfigMap", "Namespace", cd.Namespace, "Name", configMapName)
			}
		}
	}
	// find the pd secret and delete id
	r.reqLogger.Info("Deleting PD secret", "Namespace", cd.Namespace, "Name", secretName)
	err = utils.DeleteSecret(secretName, cd.Namespace, r.client, r.reqLogger)
	if err != nil {
		r.reqLogger.Error(err, "Error deleting Secret", "Namespace", cd.Namespace, "Name", secretName)
	}

	// find the PD syncset and delete it
	r.reqLogger.Info("Deleting PD SyncSet", "Namespace", cd.Namespace, "Name", secretName)
	err = utils.DeleteSyncSet(secretName, cd.Namespace, r.client, r.reqLogger)

	if err != nil {
		r.reqLogger.Error(err, "Error deleting SyncSet", "Namespace", cd.Namespace, "Name", secretName)
	}

	if utils.HasFinalizer(cd, finalizer) {
		r.reqLogger.Info("Deleting PD finalizer from ClusterDeployment", "Namespace", cd.Namespace, "Name", cd.Name)
		baseToPatch := client.MergeFrom(cd.DeepCopy())
		utils.DeleteFinalizer(cd, finalizer)
		if err := r.client.Patch(context.TODO(), cd, baseToPatch); err != nil {
			r.reqLogger.Error(err, "Error deleting Finalizer from cluster deployment", "Namespace", cd.Namespace, "Name", cd.Name)
			metrics.UpdateMetricPagerDutyDeleteFailure(1, ClusterID, pdi.Name)
			return err
		}
	}

	if utils.HasFinalizer(cd, config.LegacyPagerDutyFinalizer) {
		r.reqLogger.Info("Deleting old PD finalizer from ClusterDeployment", "Namespace", cd.Namespace, "Name", cd.Name)
		utils.DeleteFinalizer(cd, config.LegacyPagerDutyFinalizer)
		err = r.client.Update(context.TODO(), cd)
		if err != nil {
			metrics.UpdateMetricPagerDutyDeleteFailure(1, ClusterID, pdi.Name)
			return err
		}
	}

	metrics.UpdateMetricPagerDutyDeleteFailure(0, ClusterID, pdi.Name)

	return nil
}
