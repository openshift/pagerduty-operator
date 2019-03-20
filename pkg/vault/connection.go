package vault

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/vault/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetVaultSecret Gets a designed token from vault. Vault creds are stored in a k8s secret
func GetVaultSecret(osc client.Client, namespace string, name string, path string, property string) (string, error) {
	vaultConfig := &corev1.Secret{}

	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, vaultConfig)
	if err != nil {
		return "", err
	}

	vaultURL, ok := vaultConfig.Data["VAULT_URL"]
	if !ok {
		return "", errors.New("VAULT_URL is not set")
	}
	if len(vaultURL) <= 0 {
		return "", errors.New("VAULT_URL is empty")
	}

	vaultToken, ok := vaultConfig.Data["VAULT_TOKEN"]
	if !ok {
		return "", errors.New("VAULT_TOKEN is not set")
	}
	if len(vaultToken) <= 0 {
		return "", errors.New("VAULT_TOKEN is empty")
	}

	client, err := api.NewClient(&api.Config{
		Address: string(vaultURL),
	})
	if err != nil {
		return "", err
	}
	client.SetToken(string(vaultToken))

	vault, err := client.Logical().Read(path)
	if err != nil {
		return "", err
	}

	if len(vault.Data) == 0 {
		return "", errors.New("Vault data is empty")
	}

	fmt.Printf("Error: %+v\n", vault)

	for propName, propValue := range vault.Data {
		if propName == property {
			secret := fmt.Sprintf("%v", propValue)
			if len(secret) <= 0 {
				return "", errors.New(property + " is empty")
			}
			return secret, nil
		}
	}

	return "", errors.New(property + " not set in vault")
}
