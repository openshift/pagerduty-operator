package pagerdutyintegration

import (
	"strconv"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
)

func (r *PagerDutyIntegrationReconciler) handleLimitedSupport(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
	// configMapName is the name of the ConfigMap of the relevant service
	var configMapName = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)

	// check if the cluster isn't installed yet
	if !cd.Spec.Installed {
		// if the cluster hasn't been installed yet, return
		return nil
	}

	// PagerDuty data
	clusterID := utils.GetClusterID(cd)
	pdData, err := pd.NewData(pdi, clusterID, cd.Spec.BaseDomain)
	if err != nil {
		return err
	}

	err = pdData.ParseClusterConfig(r.Client, cd.Namespace, configMapName)
	if err != nil || pdData.ServiceID == "" {
		// pagerduty service isn't created yet, return
		return nil
	}

	// Check if limited-support label exists in CD
	hasLimitedSupport := false
	if val, err := strconv.ParseBool(cd.Labels[config.ClusterDeploymentLimitedSupportLabel]); err == nil {
		hasLimitedSupport = val
	}

	// Check if cluster has support exception label on ClusterDeployment
	hasSupportException := false
	if supportExValue, err := strconv.ParseBool(cd.Labels[config.ClusterDeploymentSupportExceptionLabel]); err == nil {
		hasSupportException = supportExValue
	}

	if hasSupportException && pdData.LimitedSupport {
		// Enable PagerDuty service if the cluster is in limited support
		r.reqLogger.Info("The cluster has a support exception, re-enabling PagerDuty service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain)
		if err := pdclient.EnableService(pdData); err != nil {
			r.reqLogger.Error(err, "Error re-enabling PagerDuty service")
			return err
		}
	}

	if hasLimitedSupport && !pdData.LimitedSupport {
		if hasSupportException {
			// Keep PagerDuty service active even though cluster is in limited support
			r.reqLogger.Info("The cluster has a support exception, not disabling PagerDuty service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain)
			return nil
		}
		// Disable PD service and resolve existing service alerts if limited-support label set to true
		r.reqLogger.Info("The cluster is in limited-support, disabling PagerDuty service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain)
		if err := pdclient.DisableService(pdData); err != nil {
			r.reqLogger.Error(err, "Error disabling PagerDuty service")
			return err
		}

		pdData.LimitedSupport = true

		if err := pdData.SetClusterConfig(r.Client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name", configMapName)
			return err
		}
	} else if !hasLimitedSupport && pdData.LimitedSupport {
		// Enable PagerDuty service if limited-support label is-not-true/does-not-exist
		r.reqLogger.Info("The cluster is not in limited-support, enabling PagerDuty service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain)
		if err := pdclient.EnableService(pdData); err != nil {
			r.reqLogger.Error(err, "Error enabling PagerDuty service")
			return err
		}

		pdData.LimitedSupport = false

		if err := pdData.SetClusterConfig(r.Client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name", configMapName)
			return err
		}
	}

	return nil
}
