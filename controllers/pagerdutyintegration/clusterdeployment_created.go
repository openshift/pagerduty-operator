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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	"github.com/openshift/pagerduty-operator/pkg/kube"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *PagerDutyIntegrationReconciler) handleCreate(ctx context.Context, pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
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

		// fakeClusterDeploymentAnnotation defines if the cluster is a fake cluster.
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

	if !utils.HasFinalizer(cd, finalizer) {
		baseToPatch := client.MergeFrom(cd.DeepCopy())
		utils.AddFinalizer(cd, finalizer)
		return r.Patch(ctx, cd, baseToPatch)
	}

	clusterID := utils.GetClusterID(cd)
	pdData, err := pd.NewData(pdi, clusterID, cd.Spec.BaseDomain)
	if err != nil {
		return err
	}

	// load configuration; if missing, create the PD service and configmap
	if err = pdData.ParseClusterConfig(ctx, r.Client, cd.Namespace, configMapName); err != nil || pdData.ServiceID == "" {
		if err = r.createPDServiceAndConfigMap(ctx, pdclient, pdi, cd, pdData, clusterID, configMapName); err != nil {
			return err
		}
	}

	// Sync the escalation policy
	if err = r.syncEscalationPolicy(ctx, pdclient, pdi, cd, pdData, configMapName); err != nil {
		return err
	}

	// Get or create the integration key
	pdIntegrationKey, err := r.getOrCreateIntegrationKey(ctx, pdclient, pdData, secretName, cd)
	if err != nil {
		return err
	}

	// Create / reconcile the PD Secret
	secret := kube.GeneratePdSecret(cd.Namespace, secretName, pdIntegrationKey)
	r.reqLogger.Info("creating pd secret", "ClusterDeployment.Namespace", cd.Namespace)
	if err = controllerutil.SetControllerReference(cd, secret, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on secret", "ClusterDeployment.Namespace", cd.Namespace)
		return err
	}
	if err = r.reconcilePDSecret(ctx, secret, pdIntegrationKey, cd); err != nil {
		return err
	}

	// Create the SyncSet if it doesn't exist
	return r.ensureSyncSet(ctx, pdi, cd, secret, secretName)
}

// createPDServiceAndConfigMap creates the PD service and saves its config to a ConfigMap.
func (r *PagerDutyIntegrationReconciler) createPDServiceAndConfigMap(ctx context.Context, pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment, pdData *pd.Data, clusterID, configMapName string) error {
	r.reqLogger.Info("Creating PD service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain, "ClusterDeployment.Namespace", cd.Namespace)
	if _, err := pdclient.CreateService(pdData); err != nil {
		localmetrics.UpdateMetricPagerDutyCreateFailure(1, clusterID, pdi.Name)
		return err
	}
	localmetrics.UpdateMetricPagerDutyCreateFailure(0, clusterID, pdi.Name)

	r.reqLogger.Info("Creating configmap")
	newCM := kube.GenerateConfigMap(cd.Namespace, configMapName, pdData.ServiceID, pdData.IntegrationID, pdData.EscalationPolicyID, false, pdData.ServiceOrchestrationEnabled, pdData.ServiceOrchestrationRuleApplied, pdData.AlertGroupingType, pdData.AlertGroupingTimeout)
	if err := controllerutil.SetControllerReference(cd, newCM, r.Scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on configmap")
		return err
	}
	if err := r.Create(ctx, newCM); err != nil {
		if errors.IsAlreadyExists(err) {
			if updateErr := r.Update(ctx, newCM); updateErr != nil {
				r.reqLogger.Error(updateErr, "Error updating existing configmap", "Name", configMapName)
				return updateErr
			}
			return nil
		}
		r.reqLogger.Error(err, "Error creating configmap", "Name", configMapName)
		return err
	}
	return nil
}

// syncEscalationPolicy ensures the escalation policy in PD matches the PDI spec.
func (r *PagerDutyIntegrationReconciler) syncEscalationPolicy(ctx context.Context, pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment, pdData *pd.Data, configMapName string) error {
	if pdData.EscalationPolicyID == "" {
		pdData.EscalationPolicyID = pdi.Spec.EscalationPolicy
		if err := pdData.SetClusterConfig(ctx, r.Client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name", configMapName)
			return err
		}
		return nil
	}

	if pdData.EscalationPolicyID != pdi.Spec.EscalationPolicy {
		r.reqLogger.Info("PDI EscalationPolicy changed, updating service", "ClusterID", pdData.ClusterID, "ServiceID", pdData.ServiceID, "ClusterDeployment.Namespace", cd.Namespace)
		pdData.EscalationPolicyID = pdi.Spec.EscalationPolicy
		if err := pdclient.UpdateEscalationPolicy(pdData); err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty service", "ClusterID", pdData.ClusterID, "ServiceID", pdData.ServiceID, "ClusterDeployment.Namespace", cd.Namespace)
			return err
		}
		if err := pdData.SetClusterConfig(ctx, r.Client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name", configMapName)
			return err
		}
	}
	return nil
}

// getOrCreateIntegrationKey returns the existing integration key or creates a new one.
func (r *PagerDutyIntegrationReconciler) getOrCreateIntegrationKey(ctx context.Context, pdclient pd.Client, pdData *pd.Data, secretName string, cd *hivev1.ClusterDeployment) (string, error) {
	sc := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, sc); err == nil {
		r.reqLogger.Info("pdIntegrationKey found, skipping create", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain, "ClusterDeployment.Namespace", cd.Namespace)
		return string(sc.Data[config.PagerDutySecretKey]), nil
	}
	r.reqLogger.Info("pdIntegrationKey not found, creating one", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain, "ClusterDeployment.Namespace", cd.Namespace)
	return pdclient.GetIntegrationKey(pdData)
}

// reconcilePDSecret creates or updates the PD secret as needed.
func (r *PagerDutyIntegrationReconciler) reconcilePDSecret(ctx context.Context, secret *corev1.Secret, pdIntegrationKey string, cd *hivev1.ClusterDeployment) error {
	if err := r.Create(ctx, secret); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		r.reqLogger.Info("the pd secret exist, check if pdIntegrationKey is changed or not", "ClusterDeployment.Namespace", cd.Namespace)
		sc := &corev1.Secret{}
		if err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: cd.Namespace}, sc); err != nil {
			return err
		}
		if string(sc.Data[config.PagerDutySecretKey]) != pdIntegrationKey {
			r.reqLogger.Info("pdIntegrationKey is changed, delete the secret first")
			if err = r.Delete(ctx, secret); err != nil {
				log.Info("failed to delete existing pd secret")
				return err
			}
			r.reqLogger.Info("creating pd secret", "ClusterDeployment.Namespace", cd.Namespace)
			if err = r.Create(ctx, secret); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureSyncSet creates the SyncSet if it does not already exist.
func (r *PagerDutyIntegrationReconciler) ensureSyncSet(ctx context.Context, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment, secret *corev1.Secret, secretName string) error {
	r.reqLogger.Info("Creating syncset", "ClusterDeployment.Namespace", cd.Namespace)
	ss := &hivev1.SyncSet{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, ss)
	if err != nil {
		r.reqLogger.Info("error finding the old syncset")
		if !errors.IsNotFound(err) {
			return err
		}
		r.reqLogger.Info("syncset not found , create a new one on this ")
		ss = kube.GenerateSyncSet(cd.Namespace, cd.Name, secret, pdi)
		if err = controllerutil.SetControllerReference(cd, ss, r.Scheme); err != nil {
			r.reqLogger.Error(err, "Error setting controller reference on syncset", "ClusterDeployment.Namespace", cd.Namespace)
			return err
		}
		if err := r.Create(ctx, ss); err != nil {
			return err
		}
	}
	return nil
}
