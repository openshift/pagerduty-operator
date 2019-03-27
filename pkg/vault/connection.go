package vault

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/hashicorp/vault/api"
	pdoTypes "github.com/openshift/pagerduty-operator/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("pagerduty_vault")

func queryVault(data pdoTypes.VaultData) (string, error) {
	vaultFullPath := fmt.Sprintf("%v/data/%v", data.Mount, data.Path)

	client, err := api.NewClient(&api.Config{
		Address: string(data.URL),
	})
	if err != nil {
		return "", err
	}
	client.SetToken(string(data.Token))

	vault, err := client.Logical().Read(vaultFullPath)
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
		if propName == data.Property {
			value := fmt.Sprintf("%v", propValue)
			if len(value) <= 0 {
				return "", errors.New(data.Property + " is empty")
			}
			return value, nil
		}
	}

	return "", errors.New(data.Property + " not set in vault")
}

func saveSecret(path string, value string) error {
	os.Remove(path)
	file, err := os.Create(path)
	if err != nil {
		log.Error(err, "Failed to create temp file")
		return err
	}
	_, err = file.WriteString(value)
	if err != nil {
		log.Error(err, "Failed to write to temp file")
		return err
	}

	return nil
}

func getDataKey(data map[string][]byte, key string) (string, error) {
	if _, ok := data[key]; !ok {
		errorStr := fmt.Sprintf("%v is not set.", key)
		return "", errors.New(errorStr)
	}
	retString := string(data[key])
	if len(retString) <= 0 {
		errorStr := fmt.Sprintf("%v is empty", key)
		return "", errors.New(errorStr)
	}
	return retString, nil
}

// GetVaultSecret Gets a designed token from vault. Vault creds are stored in a k8s secret
func GetVaultSecret(osc client.Client, vaultData pdoTypes.VaultData) (string, error) {
	vaultConfig := &corev1.Secret{}

	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: vaultData.Namespace, Name: vaultData.SecretName}, vaultConfig)
	if err != nil {
		return "", err
	}

	vaultData.URL, err = getDataKey(vaultConfig.Data, "VAULT_URL")
	if err != nil {
		return "", err
	}

	vaultData.Token, err = getDataKey(vaultConfig.Data, "VAULT_TOKEN")
	if err != nil {
		return "", err
	}

	vaultData.Mount, err = getDataKey(vaultConfig.Data, "VAULT_MOUNT")
	if err != nil {
		return "", err
	}

	vaultData.Key, err = getDataKey(vaultConfig.Data, "VAULT_KEY")
	if err != nil {
		return "", err
	}

	tempFilePath := fmt.Sprintf("/tmp/%v-%v", vaultData.Mount, vaultData.Property)
	tempFile, err := os.Stat(tempFilePath)
	if os.IsNotExist(err) || tempFile.ModTime().Before(time.Now().Add(time.Hour*time.Duration(-6))) {
		secret, err := queryVault(vaultData)
		if err != nil {
			return "", err
		}
		err = saveSecret(tempFilePath, secret)
		if err != nil {
			log.Error(err, "Failed to save secret")
			return secret, nil
		}
	}

	fileDat, err := ioutil.ReadFile(tempFilePath)
	if err != nil {
		log.Error(err, "Failed to read file - removing")
		os.Remove(tempFilePath)
		return queryVault(vaultData)
	}

	return string(fileDat), nil
}
