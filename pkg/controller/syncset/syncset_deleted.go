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

package syncset

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/pagerduty-operator/pkg/kube"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileSyncSet) recreateSyncSet(request reconcile.Request) (reconcile.Result, error) {
	r.reqLogger.Info("Syncset deleted, regenerating")

	clusterdeployment := &hivev1.ClusterDeployment{}
	cdName := request.Name[0 : len(request.Name)-8]
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: cdName}, clusterdeployment)
	if err != nil {
		// Error finding the cluster deployment, requeue
		return reconcile.Result{}, err
	}

	pdData := &pd.Data{
		ClusterID:  clusterdeployment.Spec.ClusterName,
		BaseDomain: clusterdeployment.Spec.BaseDomain,
	}
	pdData.ParsePDConfig(r.client)
	pdData.ParseClusterConfig(r.client, request.Namespace, cdName)

	// To prevent scoping issues in the err check below.
	var pdIntegrationKey string
	recreateCM := false

	pdIntegrationKey, err = r.pdclient.GetIntegrationKey(pdData)
	if err != nil {
		var createErr error
		pdIntegrationKey, createErr = r.pdclient.CreateService(pdData)
		if createErr != nil {
			return reconcile.Result{}, createErr
		}
		recreateCM = true
	}

	secret := kube.GeneratePdSecret(request.Namespace, "pd-secret", pdIntegrationKey)
	//add SetControllerReference
	if err = controllerutil.SetControllerReference(clusterdeployment, secret, r.scheme); err != nil {
		r.reqLogger.Error(err, "Error setting controller reference on secret")
		return reconcile.Result{}, err
	}
	//check if the secret is already there , if already there , do nothing
	sc := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: "pd-secret", Namespace: request.Namespace}, sc)
	if err != nil {
		if errors.IsNotFound(err) {
			if err = r.client.Create(context.TODO(), secret); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	newSS := kube.GenerateSyncSet(request.Namespace, clusterdeployment.Name, secret)
	oldSS := &hivev1.SyncSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: request.Name + "-pd-sync", Namespace: request.Namespace}, oldSS)
	// if old ss is already there , nothing
	if err != nil { //means not exist
		if errors.IsNotFound(err) {
			if err := r.client.Create(context.TODO(), newSS); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	if recreateCM {
		pdAPIConfigMap := &corev1.ConfigMap{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: cdName + "-pd-config"}, pdAPIConfigMap)
		if err != nil {
			if !errors.IsNotFound(err) {
				return reconcile.Result{}, err
			}
		}
		r.client.Delete(context.TODO(), pdAPIConfigMap)
		newCM := kube.GenerateConfigMap(request.Namespace, cdName, pdData.ServiceID, pdData.IntegrationID)
		if err := r.client.Create(context.TODO(), newCM); err != nil {
			if errors.IsAlreadyExists(err) {
				if updateErr := r.client.Update(context.TODO(), newCM); updateErr != nil {
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
