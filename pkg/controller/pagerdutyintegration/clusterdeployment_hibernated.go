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
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/openshift/pagerduty-operator/config"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
)

func (r *ReconcilePagerDutyIntegration) handleHibernation(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
	var (
		// configMapName is the name of the ConfigMap containing the hibernation state
		configMapName string = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)
	)

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		return nil
	}

	pdData := &pd.Data{}
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

	clusterIsHibernating := cd.Spec.PowerState == hivev1.HibernatingClusterPowerState

	if clusterIsHibernating && !pdData.Hibernating {
		r.reqLogger.Info("Disabling PD service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain)
		if err := pdclient.DisableService(pdData); err != nil {
			return err
		}
		pdData.Hibernating = true
		if err := pdData.SetClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating pd cluster config", "Name", configMapName)
			return err
		}
	} else if !clusterIsHibernating && pdData.Hibernating {
		r.reqLogger.Info("Enabling PD service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain)
		if err := pdclient.EnableService(pdData); err != nil {
			return err
		}
		pdData.Hibernating = false
		if err := pdData.SetClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating pd cluster config", "Name", configMapName)
			return err
		}
	}

	return nil
}
