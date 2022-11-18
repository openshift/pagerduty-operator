package utils

import (
	"context"
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadConfigMapData loads a given configmap key and returns its data as a string.
func LoadConfigMapData(c client.Client, configMap types.NamespacedName, dataKey string) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := c.Get(context.TODO(), configMap, cm); err != nil {
		return "", err
	}

	cmData, ok := cm.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("configmap %s did not contain key %s", configMap.Name, dataKey)
	}
	if len(cmData) == 0 {
		return "", fmt.Errorf("%s is empty", dataKey)
	}

	var js json.RawMessage
	err := json.Unmarshal([]byte(cmData), &js)
	if err != nil {
		return "", fmt.Errorf("failed to read configmap data as json: %v", err)
	}

	return cmData, nil
}
