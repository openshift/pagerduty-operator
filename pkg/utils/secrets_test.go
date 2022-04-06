package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testSecretName = "testSecret"
	testNamespace  = "testNamespace"
	testDataKey    = "testKey"
	testDataValue  = "testValue"
)

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

func testSecretEmptyData() *corev1.Secret {
	sc := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			testDataKey: {},
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
	tests := []struct {
		name        string
		secret      *corev1.Secret
		dataValue   string
		expectError bool
		expectEqual bool
	}{
		{
			name:        "Test secret with dataKey",
			secret:      testSecret(),
			dataValue:   testDataValue,
			expectError: false,
			expectEqual: true,
		},
		{
			name:        "Test secret with empty dataKey",
			secret:      testSecretEmptyData(),
			dataValue:   "",
			expectError: true,
			expectEqual: true,
		},
		{
			name:        "Test secret without dataKey",
			secret:      testSecretNoData(),
			dataValue:   "",
			expectError: true,
			expectEqual: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
			client := fake.NewFakeClientWithScheme(s, test.secret)

			result, err := LoadSecretData(client, testSecretName, testNamespace, testDataKey)
			if err != nil && !test.expectError {
				t.Errorf("Unexpected error: %v", err)
			}

			if test.expectEqual {
				assert.Equal(t, result, test.dataValue)
			} else {
				assert.NotEqual(t, result, test.dataValue)
			}
		})
	}
}
