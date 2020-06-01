// Copyright 2020 Red Hat
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
	"fmt"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/pagerduty-operator/config"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	"github.com/openshift/pagerduty-operator/pkg/kube"
	"github.com/openshift/pagerduty-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcilePagerDutyIntegration) handleMigrate(pdi *pagerdutyv1alpha1.PagerDutyIntegration, cd *hivev1.ClusterDeployment) error {
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
		return nil
	}

	// Step 1: Get old ConfigMap, Secret, SyncSet (or ignore migration if can't Get)

	oldCM := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cd.Name + config.ConfigMapSuffix, Namespace: cd.Namespace}, oldCM)
	if err != nil {
		r.reqLogger.Error(
			err, "Couldn't get legacy ConfigMap, assuming no migration to be done",
			"ClusterDeployment.Name", cd.Name, "ClusterDeployment.Namespace", cd.Namespace,
			"PagerDutyIntegration.Name", pdi.Name, "PagerDutyIntegration.Namespace", pdi.Namespace,
		)
		return nil
	}

	oldSecret := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: "pd-secret", Namespace: cd.Namespace}, oldSecret)
	if err != nil {
		r.reqLogger.Error(
			err, "Couldn't get legacy Secret, assuming no migration to be done",
			"ClusterDeployment.Name", cd.Name, "ClusterDeployment.Namespace", cd.Namespace,
			"PagerDutyIntegration.Name", pdi.Name, "PagerDutyIntegration.Namespace", pdi.Namespace,
		)
		return nil
	}

	oldSyncSet := &hivev1.SyncSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cd.Name + "-pd-sync", Namespace: cd.Namespace}, oldSyncSet)
	if err != nil {
		r.reqLogger.Error(
			err, "Couldn't get legacy SyncSet, assuming no migration to be done",
			"ClusterDeployment.Name", cd.Name, "ClusterDeployment.Namespace", cd.Namespace,
			"PagerDutyIntegration.Name", pdi.Name, "PagerDutyIntegration.Namespace", pdi.Namespace,
		)
		return nil
	}

	// Step 2: Duplicate ConfigMap, Secret, SyncSet into new names

	newCM := kube.GenerateConfigMap(cd.Namespace, configMapName, oldCM.Data["SERVICE_ID"], oldCM.Data["INTEGRATION_ID"])
	if err = controllerutil.SetControllerReference(cd, newCM, r.scheme); err != nil {
		return fmt.Errorf("Couldn't set controller reference on ConfigMap: %w", err)
	}
	if err := r.client.Create(context.TODO(), newCM); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("Couldn't create new ConfigMap: %w", err)
	}

	newSecret := &corev1.Secret{
		Type: "Opaque",
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cd.Namespace,
		},
		Data: map[string][]byte{
			"PAGERDUTY_KEY": oldSecret.Data["PAGERDUTY_KEY"],
		},
	}
	if err = controllerutil.SetControllerReference(cd, newSecret, r.scheme); err != nil {
		return fmt.Errorf("Couldn't set controller reference on Secret: %w", err)
	}
	if err = r.client.Create(context.TODO(), newSecret); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("Couldn't create new Secret: %w", err)
	}

	newSyncSet := kube.GenerateSyncSet(cd.Namespace, secretName, newSecret)
	if err = controllerutil.SetControllerReference(cd, newSyncSet, r.scheme); err != nil {
		return fmt.Errorf("Couldn't set controller reference on SyncSet: %w", err)
	}
	if err = r.client.Create(context.TODO(), newSyncSet); err != nil {
		return fmt.Errorf("Couldn't create new SyncSet: %w", err)
	}

	// Step 3: Delete old ConfigMap, Secret, SyncSet

	err = r.client.Delete(context.TODO(), oldCM)
	if err != nil {
		return fmt.Errorf("Couldn't delete legacy ConfigMap: %w", err)
	}

	err = r.client.Delete(context.TODO(), oldSyncSet)
	if err != nil {
		return fmt.Errorf("Couldn't delete legacy SyncSet: %w", err)
	}

	err = r.client.Delete(context.TODO(), oldSecret)
	if err != nil {
		return fmt.Errorf("Couldn't delete legacy Secret: %w", err)
	}

	// Step 4: Update finalizers on ClusterDeployment

	if utils.HasFinalizer(cd, finalizer) == false {
		utils.AddFinalizer(cd, finalizer)
		err := r.client.Update(context.TODO(), cd)
		if err != nil {
			return err
		}
	}

	if utils.HasFinalizer(cd, config.OperatorFinalizer) {
		utils.DeleteFinalizer(cd, config.OperatorFinalizer)
		err := r.client.Update(context.TODO(), cd)
		if err != nil {
			return err
		}
	}

	return nil
}
