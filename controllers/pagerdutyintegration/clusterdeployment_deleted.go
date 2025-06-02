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
	"fmt"
	"strings"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	metrics "github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *PagerDutyIntegrationReconciler) handleDelete(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
	if cd == nil {
		// nothing to do, bail early
		return nil
	}

	var (
		// secretName is the name of the Secret deployed to the target
		// cluster, and also the name of the SyncSet that causes it to
		// be deployed.
		secretName = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.SecretSuffix)

		// configMapName is the name of the ConfigMap containing the
		// SERVICE_ID and INTEGRATION_ID
		configMapName = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)

		// There can be more than one PagerDutyIntegration that causes
		// creation of resources for a ClusterDeployment, and each one
		// will need a finalizer here. We add a suffix of the CR
		// name to distinguish them.
		finalizer = config.PagerDutyFinalizerPrefix + pdi.Name
	)

	if !utils.HasFinalizer(cd, finalizer) {
		return nil
	}

	clusterID := utils.GetClusterID(cd)
	pdData, err := pd.NewData(pdi, clusterID, cd.Spec.BaseDomain)
	if err != nil {
		return err
	}

	// Evaluate edge-cases where the PagerDuty service no longer needs to be deleted
	deletePDService := true

	// If the Configmap containing the PagerDuty service parameters is missing, the controller has no hope
	// of deleting the service, so just cleanup the rest of the Kubernetes resources
	if err := pdData.ParseClusterConfig(r.Client, cd.Namespace, configMapName); err != nil {
		if !errors.IsNotFound(err) {
			// some error other than not found, requeue
			return err
		}
		deletePDService = false
	}

	// Check if the PD Service still exists, if not DeleteService returns errors
	if deletePDService {
		_, err = pdclient.GetService(pdData)

		if err != nil {
			if !strings.Contains(err.Error(), "Not Found") {
				return err
			}
			r.reqLogger.Info(fmt.Sprintf("PD service %s-%s.%s not found...skipping PD service deletion", pdData.ServicePrefix, pdData.ClusterID, pdData.BaseDomain))
			deletePDService = false
		}
	}

	// None of the edge cases apply, delete the PagerDuty service
	if deletePDService {
		r.reqLogger.Info(fmt.Sprintf("Deleting PD service %s-%s.%s", pdData.ServicePrefix, pdData.ClusterID, pdData.BaseDomain))
		if err := pdclient.DeleteService(pdData); err != nil {
			r.reqLogger.Error(err, "Failed cleaning up pagerduty.", "ClusterDeployment.Namespace", cd.Namespace, "ClusterID", pdData.ClusterID)
			return err
		}

		// Only delete the configmap if the PagerDuty service was successfully deleted because
		// it contains the service ID which can be used to find and delete the service next time.
		r.reqLogger.Info("Deleting PD ConfigMap", "ClusterDeployment.Namespace", cd.Namespace, "Name", configMapName)
		if err := utils.DeleteConfigMap(configMapName, cd.Namespace, r.Client, r.reqLogger); err != nil {
			r.reqLogger.Error(err, "Error deleting ConfigMap", "ClusterDeployment.Namespace", cd.Namespace, "Name", configMapName)
		}
	}

	// find the pd secret and delete id
	r.reqLogger.Info("Deleting PD secret", "ClusterDeployment.Namespace", cd.Namespace, "Name", secretName)
	err = utils.DeleteSecret(secretName, cd.Namespace, r.Client, r.reqLogger)
	if err != nil {
		r.reqLogger.Error(err, "Error deleting Secret", "ClusterDeployment.Namespace", cd.Namespace, "Name", secretName)
	}

	// find the PD syncset and delete it
	r.reqLogger.Info("Deleting PD SyncSet", "ClusterDeployment.Namespace", cd.Namespace, "Name", secretName)
	err = utils.DeleteSyncSet(secretName, cd.Namespace, r.Client, r.reqLogger)

	if err != nil {
		r.reqLogger.Error(err, "Error deleting SyncSet", "ClusterDeployment.Namespace", cd.Namespace, "Name", secretName)
	}

	if utils.HasFinalizer(cd, finalizer) {
		r.reqLogger.Info("Deleting PD finalizer from ClusterDeployment", "ClusterDeployment.Namespace", cd.Namespace, "ClusterDeployment Name", cd.Name)
		baseToPatch := client.MergeFrom(cd.DeepCopy())
		utils.DeleteFinalizer(cd, finalizer)
		err = r.Patch(context.TODO(), cd, baseToPatch);
		if err != nil {
			r.reqLogger.Error(err, "Error deleting Finalizer from cluster deployment", "ClusterDeployment.Namespace", cd.Namespace, "ClusterDeployment Name", cd.Name)
			metrics.UpdateMetricPagerDutyDeleteFailure(1, clusterID, pdi.Name)
			return err
		}
	}

	if utils.HasFinalizer(cd, config.LegacyPagerDutyFinalizer) {
		r.reqLogger.Info("Deleting old PD finalizer from ClusterDeployment", "ClusterDeployment.Namespace", cd.Namespace, "ClusterDeployment Name", cd.Name)
		utils.DeleteFinalizer(cd, config.LegacyPagerDutyFinalizer)
		err = r.Update(context.TODO(), cd)
		if err != nil {
			metrics.UpdateMetricPagerDutyDeleteFailure(1, clusterID, pdi.Name)
			return err
		}
	}

	metrics.UpdateMetricPagerDutyDeleteFailure(0, clusterID, pdi.Name)

	return nil
}
