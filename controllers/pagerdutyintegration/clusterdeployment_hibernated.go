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
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	// This can be removed once Hive is promoted past f73ed3e in all environments
	// Support for this condition was removed in https://github.com/openshift/hive/pull/1604
	legacyHivev1RunningHibernationReason = "Running"
)

func (r *PagerDutyIntegrationReconciler) handleHibernation(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
	var (
		// configMapName is the name of the ConfigMap containing the hibernation state
		configMapName string = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)
	)

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		return nil
	}

	clusterID := utils.GetClusterID(cd)
	pdData, err := pd.NewData(pdi, clusterID, cd.Spec.BaseDomain)
	if err != nil {
		return err
	}

	if err := pdData.ParseClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
		if errors.IsNotFound(err) {
			// service isn't created yet, return
			return nil
		}
		return err
	}
	if pdData.ServiceID == "" {
		// service isn't created yet, return
		return nil
	}

	specIsHibernating := cd.Spec.PowerState == hivev1.ClusterPowerStateHibernating

	if specIsHibernating && !pdData.Hibernating {
		r.reqLogger.Info("Disabling PD service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain, "ClusterDeployment.Namespace", cd.Namespace)
		if err := pdclient.DisableService(pdData); err != nil {
			return err
		}
		pdData.Hibernating = true
		if err := pdData.SetClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating pd cluster config", "Name", configMapName, "ClusterDeployment.Namespace", cd.Namespace)
			return err
		}
	} else if !specIsHibernating && pdData.Hibernating {
		if instancesAreRunning(cd) {
			r.reqLogger.Info("Enabling PD service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain, "ClusterDeployment.Namespace", cd.Namespace)
			if err := pdclient.EnableService(pdData); err != nil {
				return err
			}
			pdData.Hibernating = false
			if err := pdData.SetClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
				r.reqLogger.Error(err, "Error updating pd cluster config", "Name", configMapName, "ClusterDeployment.Namespace", cd.Namespace)
				return err
			}
		}
	}

	return nil
}

func instancesAreRunning(cd *hivev1.ClusterDeployment) bool {
	// Get hibernation PowerState a new ClusterDeployment Status field indicating if the cluster is running
	// ie. The cluster is not "Resuming" if the PowerState is "Running", the cluster is operational.
	// If the field is blank we move on and check the legacy reasons (It may be blank if the running version of
	// Hive on cluster doesn't yet support it)
	if cd.Status.PowerState == "Running" {
		return true
	}

	// This can be removed once Hive is promoted past f73ed3e in all environments
	// We can rely on ClusterDeployment.Status.PowerState
	hibernatingCondition := getCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition)

	// Verify the ClusterDeployment has a hibernation condition
	if hibernatingCondition == nil {
		return false
	}

	// Verify the hibernatingCondition is not active (ConditionTrue and ConditionUnknown are discarded)
	if hibernatingCondition.Status != corev1.ConditionFalse {
		return false
	}

	// Check legacy Hibernation condition reasons
	return hibernatingCondition.Reason == legacyHivev1RunningHibernationReason
}

func getCondition(conditions []hivev1.ClusterDeploymentCondition, t hivev1.ClusterDeploymentConditionType) *hivev1.ClusterDeploymentCondition {
	for _, condition := range conditions {
		if condition.Type == t {
			return &condition
		}
	}
	return nil
}
