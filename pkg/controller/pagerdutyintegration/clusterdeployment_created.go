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
	"github.com/openshift/pagerduty-operator/config"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	"github.com/openshift/pagerduty-operator/pkg/kube"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcilePagerDutyIntegration) handleCreate(pdclient pd.Client, pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
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
		return r.client.Patch(context.TODO(), cd, baseToPatch)
	}

	clusterID := utils.GetClusterID(cd)
	pdData, err := pd.NewData(pdi, clusterID, cd.Spec.BaseDomain)
	if err != nil {
		return err
	}

	// load configuration
	err = pdData.ParseClusterConfig(r.client, cd.Namespace, configMapName)

	if err != nil || pdData.ServiceID == "" {
		// unable to load configuration, therefore create the PD service
		var createErr error
		r.reqLogger.Info("Creating PD service", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain, "ClusterDeployment.Namespace", cd.Namespace)
		_, createErr = pdclient.CreateService(pdData)
		if createErr != nil {
			localmetrics.UpdateMetricPagerDutyCreateFailure(1, clusterID, pdi.Name)
			return createErr
		}
		localmetrics.UpdateMetricPagerDutyCreateFailure(0, clusterID, pdi.Name)

		r.reqLogger.Info("Creating configmap")

		// save config map
		newCM := kube.GenerateConfigMap(cd.Namespace, configMapName, pdData.ServiceID, pdData.IntegrationID, pdData.EscalationPolicyID, false, false)
		if err = controllerutil.SetControllerReference(cd, newCM, r.scheme); err != nil {
			r.reqLogger.Error(err, "Error setting controller reference on configmap")
			return err
		}
		if err := r.client.Create(context.TODO(), newCM); err != nil {
			if errors.IsAlreadyExists(err) {
				if updateErr := r.client.Update(context.TODO(), newCM); updateErr != nil {
					r.reqLogger.Error(err, "Error updating existing configmap", "Name", configMapName)
					return err
				}
				return nil
			}
			r.reqLogger.Error(err, "Error creating configmap", "Name", configMapName)
			return err
		}
	}

	// If no value in ConfigMap for EscalationPolicyID set it from pdi.EscalationPolicyID
	if pdData.EscalationPolicyID == "" {
		// update policy ID from PDI, it is used in next set call
		pdData.EscalationPolicyID = pdi.Spec.EscalationPolicy
		if err = pdData.SetClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
			r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name", configMapName)
			return err
		}
	} else {
		// ConfigMap has a value for EscalationPolicyID
		// Check if the value is the same EscalationPolicyID as from PDI
		if pdData.EscalationPolicyID != pdi.Spec.EscalationPolicy {
			r.reqLogger.Info("PDI EscalationPolicy changed, updating service", "ClusterID", pdData.ClusterID, "ServiceID", pdData.ServiceID, "ClusterDeployment.Namespace", cd.Namespace)
			// update policy ID from PDI, it is used in next update call
			pdData.EscalationPolicyID = pdi.Spec.EscalationPolicy
			err := pdclient.UpdateEscalationPolicy(pdData)
			if err != nil {
				r.reqLogger.Error(err, "Error updating PagerDuty service", "ClusterID", pdData.ClusterID, "ServiceID", pdData.ServiceID, "ClusterDeployment.Namespace", cd.Namespace)
				return err
			}

			// Update ConfigMap to reflect the new escalation policy changes
			if err := pdData.SetClusterConfig(r.client, cd.Namespace, configMapName); err != nil {
				r.reqLogger.Error(err, "Error updating PagerDuty cluster config", "Name", configMapName)
				return err
			}

			return nil
		}
	}

	// To prevent scoping issues in the err check below.
	var pdIntegrationKey string

	// try to load integration key (secret)
	sc := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, sc)

	if err == nil {
		// successfully loaded secret, snag the integration key
		r.reqLogger.Info("pdIntegrationKey found, skipping create", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain, "ClusterDeployment.Namespace", cd.Namespace)
		pdIntegrationKey = string(sc.Data[config.PagerDutySecretKey])
	} else {
		// unable to load an integration key, create one.
		r.reqLogger.Info("pdIntegrationKey not found, creating one", "ClusterID", pdData.ClusterID, "BaseDomain", pdData.BaseDomain, "ClusterDeployment.Namespace", cd.Namespace)
		pdIntegrationKey, err = pdclient.GetIntegrationKey(pdData)
		if err != nil {
			// unable to get an integration key
			return err
		}
	}

	//add secret part
	secret := kube.GeneratePdSecret(cd.Namespace, secretName, pdIntegrationKey)
	r.reqLogger.Info("creating pd secret", "ClusterDeployment.Namespace", cd.Namespace)
	//add reference
	if err = controllerutil.SetControllerReference(cd, secret, r.scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on secret", "ClusterDeployment.Namespace", cd.Namespace)
		return err
	}
	if err = r.client.Create(context.TODO(), secret); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}

		r.reqLogger.Info("the pd secret exist, check if pdIntegrationKey is changed or not", "ClusterDeployment.Namespace", cd.Namespace)
		sc := &corev1.Secret{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: cd.Namespace}, sc)
		if err != nil {
			return nil
		}
		if string(sc.Data[config.PagerDutySecretKey]) != pdIntegrationKey {
			r.reqLogger.Info("pdIntegrationKey is changed, delete the secret first")
			if err = r.client.Delete(context.TODO(), secret); err != nil {
				log.Info("failed to delete existing pd secret")
				return err
			}
			r.reqLogger.Info("creating pd secret", "ClusterDeployment.Namespace", cd.Namespace)
			if err = r.client.Create(context.TODO(), secret); err != nil {
				return err
			}
		}
	}

	r.reqLogger.Info("Creating syncset", "ClusterDeployment.Namespace", cd.Namespace)
	ss := &hivev1.SyncSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, ss)
	if err != nil {
		r.reqLogger.Info("error finding the old syncset")
		if !errors.IsNotFound(err) {
			return err
		}
		r.reqLogger.Info("syncset not found , create a new one on this ")
		ss = kube.GenerateSyncSet(cd.Namespace, cd.Name, secret, pdi)
		if err = controllerutil.SetControllerReference(cd, ss, r.scheme); err != nil {
			r.reqLogger.Error(err, "Error setting controller reference on syncset", "ClusterDeployment.Namespace", cd.Namespace)
			return err
		}
		if err := r.client.Create(context.TODO(), ss); err != nil {
			return err
		}
	}

	return nil
}
