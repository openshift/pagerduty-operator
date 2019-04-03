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

	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/vault"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileSyncSet) handleDelete(request reconcile.Request) (reconcile.Result, error) {
	r.reqLogger.Info("Syncset deleted, regenerating")

	vaultData := vault.Data{
		Namespace:  "sre-pagerduty-operator",
		SecretName: "vaultconfig",
		Path:       "whearn",
		Property:   "pagerduty",
	}

	vaultSecret, err := vaultData.GetVaultSecret(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	pdData := &pd.Data{
		APIKey: vaultSecret,
	}
	pdData.ParsePDConfig(r.client)
	pdServiceID, err := pdData.GetService()
	if err != nil {
		return reconcile.Result{}, err
	}

	newSS := pdData.GenerateSyncSet(request.Name, request.Namespace, pdServiceID)

	if err := r.client.Create(context.TODO(), newSS); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
