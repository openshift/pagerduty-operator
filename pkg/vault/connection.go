package vault

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/hashicorp/vault/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("pagerduty_vault")

func queryVault(vaultURL string, vaultToken string, vaultMount string, vaultPath string, vaultProperty string) (string, error) {
	vaultFullPath := fmt.Sprintf("%v/data/%v", string(vaultMount), vaultPath)

	client, err := api.NewClient(&api.Config{
		Address: string(vaultURL),
	})
	if err != nil {
		return "", err
	}
	client.SetToken(string(vaultToken))

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
		if propName == vaultProperty {
			value := fmt.Sprintf("%v", propValue)
			if len(value) <= 0 {
				return "", errors.New(vaultProperty + " is empty")
			}
			return value, nil
		}
	}

	return "", errors.New(vaultProperty + " not set in vault")
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

	tempFilePath := fmt.Sprintf("/tmp/%v-%v", string(vaultMount), path)
	tempFile, err := os.Stat(tempFilePath)
	if os.IsNotExist(err) || tempFile.ModTime().Before(time.Now().Add(time.Hour*time.Duration(-6))) {
		secret, err := queryVault(string(vaultURL), string(vaultToken), string(vaultMount), path, property)
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
		return queryVault(string(vaultURL), string(vaultToken), string(vaultMount), path, property)
	}

	return string(fileDat), nil
}
