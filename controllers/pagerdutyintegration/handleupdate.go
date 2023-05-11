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
	"github.com/openshift/pagerduty-operator/config"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
)

func (r *PagerDutyIntegrationReconciler) handleUpdate(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
	var (
		// configMapName is the name of the ConfigMap containing the
		// SERVICE_ID and INTEGRATION_ID
		configMapName string = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)
	)

	pdData, err := pd.NewData(pdi, cd.Spec.ClusterMetadata.ClusterID, cd.Spec.BaseDomain)
	if err != nil {
		return err
	}
	err = pdData.ParseClusterConfig(r.Client, cd.ObjectMeta.Namespace, configMapName)
	if err != nil {
		return err
	}
	if pdData.AlertGroupingType != pdi.Spec.AlertGroupingParameters.Type || pdData.AlertGroupingTimeout != pdi.Spec.AlertGroupingParameters.Config.Timeout {
		err = pdclient.UpdateAlertGrouping(pdData)
		if err != nil {
			return err
		}
	}
	return nil
}
