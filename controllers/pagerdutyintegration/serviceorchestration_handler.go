package pagerdutyintegration

import (
	"context"
	"fmt"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
)

const (
	ServiceOrchestrationDataName = "service-orchestration.json"
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

	pdiSelector, err := metav1.LabelSelectorAsSelector(&pdi.Spec.ClusterDeploymentSelector)
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
		Name:      pdi.Spec.ServiceOrchestration.RuleConfigConfigMapRef.Name,
		Namespace: pdi.Spec.ServiceOrchestration.RuleConfigConfigMapRef.Namespace,
	}

	ruleConfigmap := &corev1.ConfigMap{}

	err = r.Get(context.TODO(), serviceOrchestrationConfigMap, ruleConfigmap)
	if errors.IsNotFound(err) {
		r.reqLogger.Info("service orchestration configmap rule not found, skip the following steps")
		localmetrics.UpdateMetricPagerDutyServiceOrchestrationFailure(1, pdi.Name)
		return nil
	}
	if err != nil {
		return err
	}

	if pdiSelector.Matches(labels.Set(ruleConfigmap.GetLabels())) {
		orchestrationConfig, err := utils.LoadConfigMapData(r.Client,
			serviceOrchestrationConfigMap, ServiceOrchestrationDataName)
		if err != nil {
			return err
		}

		pdData.ServiceOrchestrationRules = orchestrationConfig

		r.reqLogger.Info(fmt.Sprintf("apply the service orchestration rules from configmap: %s",
			orchestrationConfigmapName))
		err = pdclient.ApplyServiceOrchestrationRule(pdData)
		if err != nil {
			return err
		}

		pdData.ServiceOrchestrationRuleApplied = true

		err = pdData.SetClusterConfig(r.Client, cd.Namespace, clusterConfigmapName)
		if err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name",
				clusterConfigmapName)
			return err
		}
	}

	return nil
}
