package pagerdutyintegration

import (
	"context"
	"fmt"
	"reflect"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	StandardServiceOrchestrationDataName    = "service-orchestration.json"
	RedHatInfraServiceOrchestrationDataName = "rh-infra-service-orchestration.json"
)

// handleServiceOrchestration enables and applies the service orchestration rule to the PD service if it is enabled in PDI
func (r *PagerDutyIntegrationReconciler) handleServiceOrchestration(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
	if reflect.ValueOf(pdi.Spec.ServiceOrchestration.RuleConfigConfigMapRef).IsZero() {
		r.reqLogger.Info("service orchestration is not defined correctly in PagerdutyIntegration, skipping...")
		return nil
	}

	var (
		// clusterConfigmapName is the name of the ConfigMap containing the
		// SERVICE_ID and INTEGRATION_ID
		clusterConfigmapName string = config.Name(pdi.Spec.ServicePrefix, cd.Name, config.ConfigMapSuffix)

		// orchestrationConfigmapName is the name of the configmap containing the
		// service orchestration rules
		orchestrationConfigmapName string = pdi.Spec.ServiceOrchestration.RuleConfigConfigMapRef.Name

		fakeClusterDeploymentAnnotation = "managed.openshift.com/fake"
	)

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		return nil
	}

	val, ok := cd.Annotations[fakeClusterDeploymentAnnotation]
	if ok && val == "true" {
		r.reqLogger.Info("Fake cluster identified: " + cd.Spec.ClusterName + ". Skipping reconcile.")
		return nil
	}

	clusterID := utils.GetClusterID(cd)
	pdData, err := pd.NewData(pdi, clusterID, cd.Spec.BaseDomain)
	if err != nil {
		return err
	}

	clusterConfigMap := &corev1.ConfigMap{}

	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: cd.Namespace, Name: clusterConfigmapName}, clusterConfigMap)
	if err != nil {
		return err
	}

	// load configuration
	err = pdData.ParseClusterConfig(r.Client, cd.Namespace, clusterConfigmapName)
	if err != nil {
		return err
	}

	if pdData.ServiceID == "" {
		// PD service has not been created, skip the service orchestration steps
		return nil
	}

	if !pdData.ServiceOrchestrationEnabled {
		r.reqLogger.Info("enabling the service orchestration")
		err = pdclient.ToggleServiceOrchestration(pdData, true)
		if err != nil {
			return err
		}

		pdData.ServiceOrchestrationEnabled = true

		err = pdData.SetClusterConfig(r.Client, cd.Namespace, clusterConfigmapName)
		if err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name",
				clusterConfigmapName)
			return err
		}
	}

	serviceOrchestrationConfigMap := types.NamespacedName{
		Name:      orchestrationConfigmapName,
		Namespace: pdi.Spec.ServiceOrchestration.RuleConfigConfigMapRef.Namespace,
	}

	orchestrationRuleConfigData := ""

	if utils.IsRedHatInfrastructure(cd) {
		orchestrationRuleConfigData, err = utils.LoadConfigMapData(r.Client,
			serviceOrchestrationConfigMap, RedHatInfraServiceOrchestrationDataName)

		if errors.IsNotFound(err) {
			r.reqLogger.Info(fmt.Sprintf("found no service orchestration configmap rule for '%s', skipping next steps", RedHatInfraServiceOrchestrationDataName))
			localmetrics.UpdateMetricPagerDutyServiceOrchestrationFailure(1, pdi.Name)
			return nil
		} else if err != nil {
			return err
		}
	} else {
		orchestrationRuleConfigData, err = utils.LoadConfigMapData(r.Client,
			serviceOrchestrationConfigMap, StandardServiceOrchestrationDataName)

		if errors.IsNotFound(err) {
			r.reqLogger.Info(fmt.Sprintf("found no service orchestration configmap rule for '%s', skipping next steps", StandardServiceOrchestrationDataName))
			localmetrics.UpdateMetricPagerDutyServiceOrchestrationFailure(1, pdi.Name)
			return nil
		} else if err != nil {
			return err
		}

	}

	if pdData.ServiceOrchestrationRuleApplied != orchestrationRuleConfigData {
		// Apply rule for new service
		pdData.ServiceOrchestrationRuleApplied = orchestrationRuleConfigData
		r.reqLogger.Info(fmt.Sprintf("applying the service orchestration rules from configmap: %s",
			orchestrationConfigmapName))
		err = pdclient.ApplyServiceOrchestrationRule(pdData)
		if err != nil {
			return err
		}

		err = pdData.SetClusterConfig(r.Client, cd.Namespace, clusterConfigmapName)
		if err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name",
				clusterConfigmapName)
			return err
		}
	} else {
		r.reqLogger.Info("applied service orchestration rule is the latest version")
	}

	return nil
}
