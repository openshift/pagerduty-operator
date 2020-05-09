package utils

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekubeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testSecretName = "testSecret"
	testNamespace  = "testNamespace"
	testDataKey    = "testKey"
	testDataValue  = "testValue"
)

func TestCheckSums(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr1 string
		jsonStr2 string
	}{
		{
			name:     "check checksum 01",
			jsonStr1: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"quay.io":{"auth":"b3BlbnNoVkc=","email":"abc@xyz.com"},"registry.connect.redhat.com":{"auth":"NjQ4ODeDZ3d1pN","email":"abc@xyz.com"},"registry.redhat.io":{"auth":"NjQ4ODX1pN","email":"abc@xyz.com"}}}`,
			jsonStr2: `{"auths":{"c.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"quay.io":{"auth":"b3BlbnNoVkc=","email":"abc@xyz.com"},"registry.connect.redhat.com":{"auth":"NjQ4ODeDZ3d1pN","email":"abc@xyz.com"},"registry.redhat.io":{"auth":"NjQ4ODX1pN","email":"abc@xyz.com"}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resultHash1 := GetHashOfPullSecret(test.jsonStr1)
			resultHash2 := GetHashOfPullSecret(test.jsonStr2)
			assert.NotEqual(t, resultHash1, resultHash2)
		})
	}
}

func testSecret() *corev1.Secret {
	sc := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			testDataKey: []byte(testDataValue),
		},
	}
	return sc
}

func testSecretNoData() *corev1.Secret {
	sc := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{},
	}
	return sc
}

func TestLoadSecretData(t *testing.T) {
	t.Run("Secret with datakey", func(t *testing.T) {
		client := fakekubeclient.NewFakeClient()
		sc := testSecret()
		_ = client.Create(context.TODO(), sc)

		result, _ := LoadSecretData(client, testSecretName, testNamespace, testDataKey)

		if result != testDataValue {
			t.Fail()
		}
	})

	t.Run("Secret without datakey", func(t *testing.T) {
		client := fakekubeclient.NewFakeClient()
		sc := testSecretNoData()
		_ = client.Create(context.TODO(), sc)

		result, _ := LoadSecretData(client, testSecretName, testNamespace, testDataKey)

		if result != "" {
			t.Fail()
		}
	})
}
