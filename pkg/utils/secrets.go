package utils

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadSecretData loads a given secret key and returns its data as a string.
func LoadSecretData(ctx context.Context, c client.Client, secretName, namespace, dataKey string) (string, error) {
	s := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, s); err != nil {
		return "", err
	}

	retStr, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	if len(retStr) == 0 {
		return "", fmt.Errorf("%s is empty", dataKey)
	}

	return string(retStr), nil
}
