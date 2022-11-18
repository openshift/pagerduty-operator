package utils

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

const (
	testConfigmapName          = "testconfigmap"
	testConfigmapDataKey       = "testcmKey"
	testConfigmapDataValue     = "testcmValue"
	testConfigmapJsonDataValue = "{\"json\": true}"
)

func testConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigmapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			testConfigmapDataKey: testConfigmapJsonDataValue,
		},
	}

	return cm
}

func testNonJsonConfigmap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigmapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			testConfigmapDataKey: testConfigmapDataValue,
		},
	}

	return cm
}

func testConfigmapEmpty() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigmapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			testConfigmapDataKey: "",
		},
	}

	return cm
}

func testConfigmapNoData() *corev1.ConfigMap {
	sc := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigmapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{},
	}
	return sc
}

func TestLoadConfigMapData(t *testing.T) {
	tests := []struct {
		name        string
		configmap   *corev1.ConfigMap
		dataValue   string
		expectError bool
		expectEqual bool
	}{
		{
			name:        "Test configmap with json dataKey",
			configmap:   testConfigMap(),
			dataValue:   testConfigmapJsonDataValue,
			expectError: false,
			expectEqual: true,
		},
		{
			name:        "Test configmap with empty dataKey",
			configmap:   testConfigmapEmpty(),
			dataValue:   "",
			expectError: true,
			expectEqual: true,
		},
		{
			name:        "Test configmap non json format dataKey",
			configmap:   testNonJsonConfigmap(),
			dataValue:   "",
			expectError: true,
			expectEqual: true,
		},
		{
			name:        "Test configmap no data",
			configmap:   testConfigmapNoData(),
			dataValue:   "",
			expectError: true,
			expectEqual: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ConfigMap{})
			client := fake.NewClientBuilder().WithScheme(s).WithObjects(test.configmap).Build()

			result, err := LoadConfigMapData(client, types.NamespacedName{Namespace: testNamespace, Name: testConfigmapName}, testConfigmapDataKey)
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
