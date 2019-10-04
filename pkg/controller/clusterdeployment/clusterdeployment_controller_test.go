package clusterdeployment

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	hiveapis "github.com/openshift/hive/pkg/apis"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	"github.com/openshift/pagerduty-operator/pkg/kube"
	mockpd "github.com/openshift/pagerduty-operator/pkg/pagerduty/mock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakekubeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testClusterName        = "testCluster"
	testNamespace          = "testNamespace"
	testIntegrationID      = "ABC123"
	testServiceID          = "DEF456"
	testAPIKey             = "test-pd-api-key"
	testEscalationPolicy   = "test-escalation-policy"
	testResolveTimeout     = "300"
	testAcknowledgeTimeout = "300"
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

//rawToSecret takes a SyncSet resource and returns the decoded Secret it contains.
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

// testPDConfigSecret creates a fake secret containing pagerduty config details to use for testing.
func testPDConfigSecret() *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: config.OperatorNamespace,
			Name:      config.PagerDutyAPISecretName,
		},
		Data: map[string][]byte{
			"PAGERDUTY_API_KEY":   []byte(testAPIKey),
			"ESCALATION_POLICY":   []byte(testEscalationPolicy),
			"RESOLVE_TIMEOUT":     []byte(testResolveTimeout),
			"ACKNOWLEDGE_TIMEOUT": []byte(testAcknowledgeTimeout),
		},
	}
	return s
}

// testPDConfigMap returns a fake configmap for a deployed cluster for testing.
func testPDConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testClusterName + "-pd-config",
		},
		Data: map[string]string{
			"INTEGRATION_ID": testIntegrationID,
			"SERVICE_ID":     testServiceID,
		},
	}
	return cm
}

// testSecret returns a Secret that will go in the SyncSet for a deployed cluster to use in testing.
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

// testSyncSet returns a SyncSet for an existing testClusterDeployment to use in testing.
func testSyncSet() *hivev1alpha1.SyncSet {
	ss := kube.GenerateSyncSet(testNamespace, testClusterName+"-pd-sync", testIntegrationID)
	return ss
}

// testClusterDeployment returns a fake ClusterDeployment for an installed cluster to use in testing.
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

// testClusterDeployment returns a fake ClusterDeployment for an installed cluster to use in testing.
func testNoalertsClusterDeployment() *hivev1alpha1.ClusterDeployment {
	labelMap := map[string]string{
		config.ClusterDeploymentManagedLabel:  "true",
		config.ClusterDeploymentNoalertsLabel: "",
	}
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

// deletedClusterDeployment returns a fake deleted ClusterDeployment to use in testing.
func deletedClusterDeployment() *hivev1alpha1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	cd.SetFinalizers([]string{config.OperatorFinalizer})

	return cd
}

// unmanagedClusterDeployment returns a fake ClusterDeployment labelled with "api.openshift.com/managed = False" to use in testing.
func unmanagedClusterDeployment() *hivev1alpha1.ClusterDeployment {
	labelMap := map[string]string{config.ClusterDeploymentManagedLabel: "false"}
	cd := testClusterDeployment()
	cd.SetLabels(labelMap)
	return cd
}

// unlabelledClusterDeployment returns a fake ClusterDeployment with no "api.openshift.com/managed" label present to use in testing.
func unlabelledClusterDeployment() *hivev1alpha1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.SetLabels(map[string]string{})
	return cd
}

// uninstalledClusterDeployment returns a ClusterDeployment with Status.installed == false to use in testing.
func uninstalledClusterDeployment() *hivev1alpha1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Status.Installed = false

	return cd
}

func TestReconcileClusterDeployment(t *testing.T) {
	hiveapis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name             string
		localObjects     []runtime.Object
		expectedSyncSets *SyncSetEntry
		verifySyncSets   func(client.Client, *SyncSetEntry) bool
		setupPDMock      func(*mockpd.MockClientMockRecorder)
	}{

		{
			name: "Test Creating",
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
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(1)
			},
		},
		{
			name: "Test Deleting",
			localObjects: []runtime.Object{
				deletedClusterDeployment(),
				testPDConfigSecret(),
				testPDConfigMap(),
			},
			expectedSyncSets: &SyncSetEntry{},
			verifySyncSets:   verifyNoSyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
				r.DeleteService(gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "Test Creating (unmanaged with label)",
			localObjects: []runtime.Object{
				unmanagedClusterDeployment(),
			},
			expectedSyncSets: &SyncSetEntry{},
			verifySyncSets:   verifyNoSyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
			},
		},
		{
			name: "Test Creating (unmanaged without label)",
			localObjects: []runtime.Object{
				unlabelledClusterDeployment(),
			},
			expectedSyncSets: &SyncSetEntry{},
			verifySyncSets:   verifyNoSyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
			},
		},
		{
			name: "Test Creating (managed with noalerts)",
			localObjects: []runtime.Object{
				testNoalertsClusterDeployment(),
			},
			expectedSyncSets: &SyncSetEntry{},
			verifySyncSets:   verifyNoSyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
			},
		},
		{
			name: "Test Uninstalled Cluster",
			localObjects: []runtime.Object{
				uninstalledClusterDeployment(),
			},
			expectedSyncSets: &SyncSetEntry{},
			verifySyncSets:   verifyNoSyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
			},
		},
		{
			name: "Test Updating",
			localObjects: []runtime.Object{
				testClusterDeployment(),
				testSecret(),
				testSyncSet(),
				testPDConfigMap(),
				testPDConfigSecret(),
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t, test.localObjects)
			test.setupPDMock(mocks.mockPDClient.EXPECT())

			defer mocks.mockCtrl.Finish()

			rcd := &ReconcileClusterDeployment{
				client:   mocks.fakeKubeClient,
				scheme:   scheme.Scheme,
				pdclient: mocks.mockPDClient,
			}

			// Act
			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testClusterName,
					Namespace: testNamespace,
				},
			})

			// Assert
			assert.NoError(t, err, "Unexpected Error")
			assert.True(t, test.verifySyncSets(mocks.fakeKubeClient, test.expectedSyncSets))
		})
	}
}

func TestRemoveAlertsAfterCreate(t *testing.T) {
	t.Run("Test Managed Cluster that later sets noalerts label", func(t *testing.T) {
		// Arrange
		mocks := setupDefaultMocks(t, []runtime.Object{
			testClusterDeployment(),
			testSecret(),
			testPDConfigSecret(),
			testPDConfigMap(), // <-- see comment below
		})
		// in order to test the delete, we need to crete the pd secret w/ a non-empty SERVICE_ID, which means CreateService won't be called

		setupDMSMock :=
			func(r *mockpd.MockClientMockRecorder) {
				// create (without calling PD)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(1)

				// delete
				r.DeleteService(gomock.Any()).Return(nil).Times(1)
			}

		setupDMSMock(mocks.mockPDClient.EXPECT())

		defer mocks.mockCtrl.Finish()

		rcd := &ReconcileClusterDeployment{
			client:   mocks.fakeKubeClient,
			scheme:   scheme.Scheme,
			pdclient: mocks.mockPDClient,
		}

		// Act (create)
		_, err := rcd.Reconcile(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      testClusterName,
				Namespace: testNamespace,
			},
		})

		// UPDATE (noalerts)
		// can't set to empty string, it won't update.. value does not matter
		clusterDeployment := &hivev1alpha1.ClusterDeployment{}
		err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testClusterName}, clusterDeployment)
		clusterDeployment.Labels[config.ClusterDeploymentNoalertsLabel] = "X"
		err = mocks.fakeKubeClient.Update(context.TODO(), clusterDeployment)

		err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testClusterName}, clusterDeployment)

		// Act (delete)
		_, err = rcd.Reconcile(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      testClusterName,
				Namespace: testNamespace,
			},
		})

		// Assert
		assert.NoError(t, err, "Unexpected Error")
		assert.True(t, verifyNoSyncSetExists(mocks.fakeKubeClient, &SyncSetEntry{}))
	})
}

// verifySyncSetExists verifies that a SyncSet exists that matches the supplied expected SyncSetEntry.
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

// verifyNoSyncSetExists verifies that there is no SyncSet present that matches the supplied expected SyncSetEntry.
func verifyNoSyncSetExists(c client.Client, expected *SyncSetEntry) bool {
	ss := hivev1alpha1.SyncSet{}
	err := c.Get(context.TODO(),
		types.NamespacedName{Name: expected.name, Namespace: testNamespace},
		&ss)
	if errors.IsNotFound(err) {
		return true
	}
	return false
}
