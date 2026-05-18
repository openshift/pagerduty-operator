package utils

import (
	"os"
	"testing"

	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/pagerduty-operator/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHasFinalizer(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []string
		check      string
		expected   bool
	}{
		{
			name:       "Has the target finalizer",
			finalizers: []string{"foo", "bar", "baz"},
			check:      "bar",
			expected:   true,
		},
		{
			name:       "Does not have the target finalizer",
			finalizers: []string{"foo", "baz"},
			check:      "bar",
			expected:   false,
		},
		{
			name:       "No finalizers",
			finalizers: nil,
			check:      "bar",
			expected:   false,
		},
		{
			name:       "Only the target finalizer",
			finalizers: []string{"bar"},
			check:      "bar",
			expected:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			obj := &metav1.ObjectMeta{Finalizers: test.finalizers}
			result := HasFinalizer(obj, test.check)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestAddFinalizer(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []string
		add        string
		expected   int
	}{
		{
			name:       "Add to empty",
			finalizers: nil,
			add:        "foo",
			expected:   1,
		},
		{
			name:       "Add alongside existing",
			finalizers: []string{"bar"},
			add:        "foo",
			expected:   2,
		},
		{
			name:       "Add duplicate",
			finalizers: []string{"foo"},
			add:        "foo",
			expected:   1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			obj := &metav1.ObjectMeta{Finalizers: test.finalizers}
			AddFinalizer(obj, test.add)
			assert.Equal(t, test.expected, len(obj.GetFinalizers()))
			assert.True(t, HasFinalizer(obj, test.add))
		})
	}
}

func TestDeleteFinalizer(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []string
		remove     string
		expected   int
	}{
		{
			name:       "Delete existing finalizer",
			finalizers: []string{"foo", "bar"},
			remove:     "foo",
			expected:   1,
		},
		{
			name:       "Delete non-existent finalizer",
			finalizers: []string{"foo", "bar"},
			remove:     "baz",
			expected:   2,
		},
		{
			name:       "Delete from empty",
			finalizers: nil,
			remove:     "foo",
			expected:   0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			obj := &metav1.ObjectMeta{Finalizers: test.finalizers}
			DeleteFinalizer(obj, test.remove)
			assert.Equal(t, test.expected, len(obj.GetFinalizers()))
			assert.False(t, HasFinalizer(obj, test.remove))
		})
	}
}

func TestDeleteConfigMap(t *testing.T) {
	tests := []struct {
		name      string
		existing  bool
		expectErr bool
	}{
		{
			name:      "ConfigMap exists",
			existing:  true,
			expectErr: false,
		},
		{
			name:      "ConfigMap does not exist",
			existing:  false,
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ConfigMap{})

			builder := fake.NewClientBuilder().WithScheme(s)
			if test.existing {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cm",
						Namespace: testNamespace,
					},
				}
				builder = builder.WithObjects(cm)
			}
			client := builder.Build()

			err := DeleteConfigMap("test-cm", testNamespace, client, logr.Discard())
			if test.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestDeleteSyncSet(t *testing.T) {
	tests := []struct {
		name      string
		existing  bool
		expectErr bool
	}{
		{
			name:      "SyncSet exists",
			existing:  true,
			expectErr: false,
		},
		{
			name:      "SyncSet does not exist",
			existing:  false,
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ConfigMap{})
			_ = hivev1.AddToScheme(s)

			builder := fake.NewClientBuilder().WithScheme(s)
			if test.existing {
				ss := &hivev1.SyncSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-syncset",
						Namespace: testNamespace,
					},
				}
				builder = builder.WithObjects(ss)
			}
			client := builder.Build()

			err := DeleteSyncSet("test-syncset", testNamespace, client, logr.Discard())
			if test.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestDeleteSecret(t *testing.T) {
	tests := []struct {
		name      string
		existing  bool
		expectErr bool
	}{
		{
			name:      "Secret exists",
			existing:  true,
			expectErr: false,
		},
		{
			name:      "Secret does not exist",
			existing:  false,
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})

			builder := fake.NewClientBuilder().WithScheme(s)
			if test.existing {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: testNamespace,
					},
				}
				builder = builder.WithObjects(secret)
			}
			client := builder.Build()

			err := DeleteSecret("test-secret", testNamespace, client, logr.Discard())
			if test.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestGetClusterID(t *testing.T) {
	t.Run("Non-fedramp returns ClusterName", func(t *testing.T) {
		os.Unsetenv("FEDRAMP")
		_ = config.SetIsFedramp()

		cd := &hivev1.ClusterDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cd",
				Namespace: "uhc-production-abc123",
			},
			Spec: hivev1.ClusterDeploymentSpec{
				ClusterName: "my-cluster",
			},
		}

		result := GetClusterID(cd)
		assert.Equal(t, "my-cluster", result)
	})

	t.Run("Fedramp returns namespace suffix", func(t *testing.T) {
		t.Setenv("FEDRAMP", "true")
		_ = config.SetIsFedramp()
		t.Cleanup(func() {
			_ = config.SetIsFedramp()
		})

		cd := &hivev1.ClusterDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cd",
				Namespace: "uhc-production-abc123",
			},
			Spec: hivev1.ClusterDeploymentSpec{
				ClusterName: "my-cluster",
			},
		}

		result := GetClusterID(cd)
		assert.Equal(t, "abc123", result)
	})
}

func TestIsRedHatInfrastructure(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "Label set to true",
			labels:   map[string]string{"ext-pagerduty.openshift.io/rh-infra": "true"},
			expected: true,
		},
		{
			name:     "Label set to false",
			labels:   map[string]string{"ext-pagerduty.openshift.io/rh-infra": "false"},
			expected: false,
		},
		{
			name:     "No label",
			labels:   map[string]string{},
			expected: false,
		},
		{
			name:     "Nil labels",
			labels:   nil,
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cd := &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cd",
					Labels: test.labels,
				},
			}
			result := IsRedHatInfrastructure(cd)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		search   string
		expected bool
	}{
		{
			name:     "Found",
			slice:    []string{"a", "b", "c"},
			search:   "b",
			expected: true,
		},
		{
			name:     "Not found",
			slice:    []string{"a", "b", "c"},
			search:   "d",
			expected: false,
		},
		{
			name:     "Empty slice",
			slice:    []string{},
			search:   "a",
			expected: false,
		},
		{
			name:     "Empty string in slice",
			slice:    []string{"", "a"},
			search:   "",
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Contains(test.slice, test.search)
			assert.Equal(t, test.expected, result)
		})
	}
}
