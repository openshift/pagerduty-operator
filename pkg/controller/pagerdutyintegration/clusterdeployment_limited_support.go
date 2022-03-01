package pagerdutyintegration

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/pagerduty-operator/config"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
)

func (r *ReconcilePagerDutyIntegration) handleLimitedSupport(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
	// configMapName is the name of the ConfigMap of the relevant service
	var configMapName = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)

	// check if the cluster isn't installed yet
	if !cd.Spec.Installed {
		// if the cluster hasn't been installed yet, return
		return nil
	}

	// PagerDuty data
	pdData := &pd.Data{}
	err := pdData.ParseClusterConfig(r.client, cd.Namespace, configMapName)
	if err != nil || pdData.ServiceID == "" {
		// pagerduty service isn't created yet, return
		return nil
	}

	// Check if limited-support label exists in CD
	hasLimitedSupport, ok := cd.Labels[config.ClusterDeploymentLimitedSupportLabel]

	if ok && hasLimitedSupport == "true" {
		// Disable pagerduty service and resolve existing service alerts if limited-support label set to true
		r.reqLogger.Info("Limited-support set to true, disabling pagerduty service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain)
		if err := pdclient.DisableService(pdData); err != nil {
			r.reqLogger.Error(err, "Error disabling pagerduty service")
			return err
		}
		if err := pdData.SetClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating cluster config", "Name", configMapName)
			return err
		}
	} else if ok {
		// Re-enable the pagerduty service if limited-support label set to false
		r.reqLogger.Info("Limited-support set to false, enabling PD service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain)
		if err := pdclient.EnableService(pdData); err != nil {
			r.reqLogger.Error(err, "Error enabling pagerduty service")
			return err
		}
		if err := pdData.SetClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating pd cluster config", "Name", configMapName)
			return err
		}
	}

	return nil
}
