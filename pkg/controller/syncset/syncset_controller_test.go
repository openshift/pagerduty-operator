package syncset

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	hiveapis "github.com/openshift/hive/pkg/apis"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"

	mockpd "github.com/openshift/pagerduty-operator/pkg/pagerduty/mock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakekubeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testClusterName   = "testCluster"
	testNamespace     = "testNamespace"
	testIntegrationID = "ABC123"
)

type SyncSetEntry struct {
	name                     string
	pdIntegrationID          string
	clusterDeploymentRefName string
}

type mocks struct {
	fakeKubeClient client.Client
	mockCtrl       *gomock.Controller
	mockPDClient   *mockpd.MockClient
}

func rawToSecret(raw runtime.RawExtension) *corev1.Secret {
	decoder := scheme.Codecs.UniversalDecoder(corev1.SchemeGroupVersion)

	obj, _, err := decoder.Decode(raw.Raw, nil, nil)
	if err != nil {
		// okay, not everything in the syncset is necessarily a secret
		return nil
	}
	s, ok := obj.(*corev1.Secret)
	if ok {
		return s
	}

	return nil
}

func setupDefaultMocks(t *testing.T, localObjects []runtime.Object) *mocks {
	mocks := &mocks{
		fakeKubeClient: fakekubeclient.NewFakeClient(localObjects...),
		mockCtrl:       gomock.NewController(t),
	}

	mocks.mockPDClient = mockpd.NewMockClient(mocks.mockCtrl)

	return mocks
}

// return a managed ClusterDeployment
func testClusterDeployment() *hivev1alpha1.ClusterDeployment {
	labelMap := map[string]string{config.ClusterDeploymentManagedLabel: "true"}
	cd := hivev1alpha1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testNamespace,
			Labels:    labelMap,
		},
		Spec: hivev1alpha1.ClusterDeploymentSpec{
			ClusterName: testClusterName,
		},
	}
	cd.Status.Installed = true

	return &cd
}

// return a Secret that will go in the SyncSet for the deployed cluster
func testSecret() *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pd-secret",
			Namespace: "openshift-monitoring",
		},
		Data: map[string][]byte{
			"PAGERDUTY_KEY": []byte(testIntegrationID),
		},
	}
	return s
}

func TestReconcileSyncSet(t *testing.T) {
	hiveapis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name             string
		localObjects     []runtime.Object
		expectedSyncSets *SyncSetEntry
		verifySyncSets   func(client.Client, *SyncSetEntry) bool
		setupPDMock      func(*mockpd.MockClientMockRecorder)
	}{
		{
			name: "Test Recreating when integration already exists in PagerDuty",
			localObjects: []runtime.Object{
				testClusterDeployment(),
				testSecret(),
			},
			expectedSyncSets: &SyncSetEntry{
				name:                     testClusterName + "-pd-sync",
				pdIntegrationID:          testIntegrationID,
				clusterDeploymentRefName: testClusterName,
			},
			verifySyncSets: verifySyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(1)
			},
		},
		{
			name: "Test Recreating when integration doesn't exist in PagerDuty",
			localObjects: []runtime.Object{
				testClusterDeployment(),
				testSecret(),
			},
			expectedSyncSets: &SyncSetEntry{
				name:                     testClusterName + "-pd-sync",
				pdIntegrationID:          testIntegrationID,
				clusterDeploymentRefName: testClusterName,
			},
			verifySyncSets: verifySyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(1)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, errors.New("Integration not found")).Times(1)
			},
		},
		{
			name: "Test SyncSet with no matching ClusterDeployment",
			localObjects: []runtime.Object{
				testSecret(),
			},
			expectedSyncSets: &SyncSetEntry{
				name:                     testClusterName + "-pd-sync",
				pdIntegrationID:          testIntegrationID,
				clusterDeploymentRefName: testClusterName,
			},
			verifySyncSets: verifyNoSyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t, test.localObjects)
			test.setupPDMock(mocks.mockPDClient.EXPECT())

			defer mocks.mockCtrl.Finish()

			rss := &ReconcileSyncSet{
				client:   mocks.fakeKubeClient,
				scheme:   scheme.Scheme,
				pdclient: mocks.mockPDClient,
			}

			// Act
			_, err := rss.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testClusterName + "-pd-sync",
					Namespace: testNamespace,
				},
			})

			// Assert
			assert.NoError(t, err, "Unexpected Error")
			assert.True(t, test.verifySyncSets(mocks.fakeKubeClient, test.expectedSyncSets))
		})
	}
}

func verifySyncSetExists(c client.Client, expected *SyncSetEntry) bool {
	ss := hivev1alpha1.SyncSet{}
	err := c.Get(context.TODO(),
		types.NamespacedName{Name: expected.name, Namespace: testNamespace},
		&ss)
	if err != nil {
		return false
	}

	if expected.name != ss.Name {
		return false
	}

	if expected.clusterDeploymentRefName != ss.Spec.ClusterDeploymentRefs[0].Name {
		return false
	}
	secret := rawToSecret(ss.Spec.Resources[0])
	if secret == nil {
		return false
	}

	return string(secret.Data["PAGERDUTY_KEY"]) == expected.pdIntegrationID
}

func verifyNoSyncSetExists(c client.Client, expected *SyncSetEntry) bool {
	ss := hivev1alpha1.SyncSet{}
	err := c.Get(context.TODO(),
		types.NamespacedName{Name: expected.name, Namespace: testNamespace},
		&ss)
	if kubeerrors.IsNotFound(err) {
		return true
	}
	return false
}
