package vault

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/vault/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("pagerduty_vault")

// GetVaultSecret Gets a designed token from vault. Vault creds are stored in a k8s secret
func GetVaultSecret(osc client.Client, namespace string, secretname string, path string, property string) (string, error) {
	vaultConfig := &corev1.Secret{}

	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: secretname}, vaultConfig)
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

	vaultMount, ok := vaultConfig.Data["VAULT_MOUNT"]
	if !ok {
		return "", errors.New("VAULT_MOUNT is not set")
	}
	if len(vaultMount) <= 0 {
		return "", errors.New("VAULT_MOUNT is empty")
	}
	fmt.Println(string(vaultMount))

	client, err := api.NewClient(&api.Config{
		Address: string(vaultURL),
	})
	if err != nil {
		return "", err
	}
	client.SetToken(string(vaultToken))

	vaultPath := fmt.Sprintf("%v/data/%v", string(vaultMount), path)

	vault, err := client.Logical().Read(vaultPath)
	if err != nil {
		return "", err
	}

	secret, ok := vault.Data["data"].(map[string]interface{})
	if !ok {
		return "", errors.New("Error parsing secret data")
	}

	if len(vault.Warnings) > 0 {
		for i := len(vault.Warnings) - 1; i >= 0; i-- {
			log.Info(vault.Warnings[i])
		}
	}

	if len(vault.Data) == 0 {
		return "", errors.New("Vault data is empty")
	}

	for propName, propValue := range secret {
		if propName == property {
			value := fmt.Sprintf("%v", propValue)
			if len(value) <= 0 {
				return "", errors.New(property + " is empty")
			}
			return value, nil
		}
	}

	return "", errors.New(property + " not set in vault")
}
