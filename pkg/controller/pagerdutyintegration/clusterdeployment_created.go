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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/pagerduty-operator/config"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	"github.com/openshift/pagerduty-operator/pkg/kube"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcilePagerDutyIntegration) handleCreate(pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) (reconcile.Result, error) {
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
		finalizer string = "pd.managed.openshift.io/" + pdi.Name
	)

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		return reconcile.Result{}, nil
	}

	if utils.HasFinalizer(cd, finalizer) == false {
		utils.AddFinalizer(cd, finalizer)
		err := r.client.Update(context.TODO(), cd)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	ClusterID := cd.Spec.ClusterName

	pdAPISecret := &corev1.Secret{}
	err := r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      pdi.Spec.PagerdutyApiKeySecretRef.Name,
			Namespace: pdi.Spec.PagerdutyApiKeySecretRef.Namespace,
		},
		pdAPISecret,
	)
	if err != nil {
		return reconcile.Result{}, err
	}

	apiKey, err := pd.GetSecretKey(pdAPISecret.Data, config.PagerDutyAPISecretKey)
	if err != nil {
		return reconcile.Result{}, err
	}

	pdData := &pd.Data{
		ClusterID:          cd.Spec.ClusterName,
		BaseDomain:         cd.Spec.BaseDomain,
		EscalationPolicyID: pdi.Spec.EscalationPolicy,
		AutoResolveTimeout: pdi.Spec.ResolveTimeout,
		AcknowledgeTimeOut: pdi.Spec.AcknowledgeTimeout,
		ServicePrefix:      pdi.Spec.ServicePrefix,
		APIKey:             apiKey,
	}

	// To prevent scoping issues in the err check below.
	var pdIntegrationKey string

	err = pdData.ParseClusterConfig(r.client, cd.Namespace, configMapName)
	if err != nil {
		var createErr error
		_, createErr = r.pdclient.CreateService(pdData)
		if createErr != nil {
			localmetrics.UpdateMetricPagerDutyCreateFailure(1, ClusterID)
			return reconcile.Result{}, createErr
		}
	}
	localmetrics.UpdateMetricPagerDutyCreateFailure(0, ClusterID)

	pdIntegrationKey, err = r.pdclient.GetIntegrationKey(pdData)
	if err != nil {
		return reconcile.Result{}, err
	}

	//add secret part
	secret := kube.GeneratePdSecret(cd.Namespace, secretName, pdIntegrationKey)
	r.reqLogger.Info("creating pd secret")
	//add reference
	if err = controllerutil.SetControllerReference(cd, secret, r.scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on secret")
		return reconcile.Result{}, err
	}
	if err = r.client.Create(context.TODO(), secret); err != nil {
		if !errors.IsAlreadyExists(err) {
			return reconcile.Result{}, err
		}

		r.reqLogger.Info("the pd secret exist, check if pdIntegrationKey is changed or not")
		sc := &corev1.Secret{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: cd.Namespace}, sc)
		if err != nil {
			return reconcile.Result{}, nil
		}
		if string(sc.Data["PAGERDUTY_KEY"]) != pdIntegrationKey {
			r.reqLogger.Info("pdIntegrationKey is changed, delete the secret first")
			if err = r.client.Delete(context.TODO(), secret); err != nil {
				log.Info("failed to delete existing pd secret")
				return reconcile.Result{}, err
			}
			r.reqLogger.Info("creating pd secret")
			if err = r.client.Create(context.TODO(), secret); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	r.reqLogger.Info("Creating syncset")
	ss := &hivev1.SyncSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, ss)
	if err != nil {
		r.reqLogger.Info("error finding the old syncset")
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		r.reqLogger.Info("syncset not found , create a new one on this ")
		ss = kube.GenerateSyncSet(cd.Namespace, secretName, secret)
		if err = controllerutil.SetControllerReference(cd, ss, r.scheme); err != nil {
			r.reqLogger.Error(err, "Error setting controller reference on syncset")
			return reconcile.Result{}, err
		}
		if err := r.client.Create(context.TODO(), ss); err != nil {
			return reconcile.Result{}, err
		}
	}

	r.reqLogger.Info("Creating configmap")
	newCM := kube.GenerateConfigMap(cd.Namespace, configMapName, pdData.ServiceID, pdData.IntegrationID)
	if err = controllerutil.SetControllerReference(cd, newCM, r.scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on configmap")
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newCM); err != nil {
		if errors.IsAlreadyExists(err) {
			if updateErr := r.client.Update(context.TODO(), newCM); updateErr != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
