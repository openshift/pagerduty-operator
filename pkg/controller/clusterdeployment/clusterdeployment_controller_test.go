package clusterdeployment

import (
	"context"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	hiveapis "github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
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
	testClusterName          = "testCluster"
	testNamespace            = "testNamespace"
	testIntegrationID        = "ABC123"
	testServiceID            = "DEF456"
	testAPIKey               = "test-pd-api-key"
	testEscalationPolicy     = "test-escalation-policy"
	testResolveTimeout       = "300"
	testAcknowledgeTimeout   = "300"
	testOtherSyncSetPostfix  = "-something-else"
	testsecretReferencesName = "pd-secret"
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
			config.PagerDutyAPISecretKey: []byte(testAPIKey),
			"ESCALATION_POLICY":          []byte(testEscalationPolicy),
			"RESOLVE_TIMEOUT":            []byte(testResolveTimeout),
			"ACKNOWLEDGE_TIMEOUT":        []byte(testAcknowledgeTimeout),
		},
	}
	return s
}

// testPDConfigMap returns a fake configmap for a deployed cluster for testing.
func testPDConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testClusterName + config.ConfigMapPostfix,
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
func testSyncSet() *hivev1.SyncSet {
	secret := kube.GeneratePdSecret(testNamespace, config.PagerDutySecretName, testIntegrationID)
	ss := kube.GenerateSyncSet(testNamespace, testClusterName+config.SyncSetPostfix, secret)
	return ss
}

// testOtherSyncSet returns a SyncSet that is not for PD for an existing testClusterDeployment to use in testing.
func testOtherSyncSet() *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName + testOtherSyncSetPostfix,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testClusterName,
				},
			},
		},
	}
}

// testClusterDeployment returns a fake ClusterDeployment for an installed cluster to use in testing.
func testClusterDeployment() *hivev1.ClusterDeployment {
	labelMap := map[string]string{config.ClusterDeploymentManagedLabel: "true"}
	cd := hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testNamespace,
			Labels:    labelMap,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testClusterName,
		},
	}
	cd.Spec.Installed = true

	return &cd
}

// testClusterDeployment returns a fake ClusterDeployment for an installed cluster to use in testing.
func testNoalertsClusterDeployment() *hivev1.ClusterDeployment {
	labelMap := map[string]string{
		config.ClusterDeploymentManagedLabel:  "true",
		config.ClusterDeploymentNoalertsLabel: "true",
	}
	cd := hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testNamespace,
			Labels:    labelMap,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testClusterName,
		},
	}
	cd.Spec.Installed = true

	return &cd
}

// deletedClusterDeployment returns a fake deleted ClusterDeployment to use in testing.
func deletedClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	cd.SetFinalizers([]string{config.OperatorFinalizer})

	return cd
}

// unmanagedClusterDeployment returns a fake ClusterDeployment labelled with "api.openshift.com/managed = False" to use in testing.
func unmanagedClusterDeployment() *hivev1.ClusterDeployment {
	labelMap := map[string]string{config.ClusterDeploymentManagedLabel: "false"}
	cd := testClusterDeployment()
	cd.SetLabels(labelMap)
	return cd
}

// unlabelledClusterDeployment returns a fake ClusterDeployment with no "api.openshift.com/managed" label present to use in testing.
func unlabelledClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.SetLabels(map[string]string{})
	return cd
}

// uninstalledClusterDeployment returns a ClusterDeployment with Spec.Installed == false to use in testing.
func uninstalledClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Installed = false

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
				name:                     testClusterName + config.SyncSetPostfix,
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
			name: "Test Deleting with missing ConfigMap",
			localObjects: []runtime.Object{
				deletedClusterDeployment(),
				testPDConfigSecret(),
			},
			expectedSyncSets: &SyncSetEntry{},
			verifySyncSets:   verifyNoSyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
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
				name:                     testClusterName + config.SyncSetPostfix,
				pdIntegrationID:          testIntegrationID,
				clusterDeploymentRefName: testClusterName,
			},
			verifySyncSets: verifySyncSetExists,
			setupPDMock: func(r *mockpd.MockClientMockRecorder) {
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(1)
			},
		},
		{
			name: "Test Creating with other SyncSets (managed with noalerts)",
			localObjects: []runtime.Object{
				testNoalertsClusterDeployment(),
				testOtherSyncSet(),
			},
			expectedSyncSets: &SyncSetEntry{name: testClusterName + testOtherSyncSetPostfix, clusterDeploymentRefName: testClusterName},
			verifySyncSets:   verifyNoSyncSetExists,
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
			testOtherSyncSet(),
			testPDConfigSecret(),
			testPDConfigMap(), // <-- see comment below
		})
		// in order to test the delete, we need to create the pd secret w/ a non-empty SERVICE_ID, which means CreateService won't be called

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
		clusterDeployment := &hivev1.ClusterDeployment{}
		err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testClusterName}, clusterDeployment)
		clusterDeployment.Labels[config.ClusterDeploymentNoalertsLabel] = "true"
		err = mocks.fakeKubeClient.Update(context.TODO(), clusterDeployment)

		err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testClusterName}, clusterDeployment)

		// Act (delete) [2x because was seeing other SyncSet's getting deleted]
		_, err = rcd.Reconcile(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      testClusterName,
				Namespace: testNamespace,
			},
		})
		_, err = rcd.Reconcile(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      testClusterName,
				Namespace: testNamespace,
			},
		})

		// Assert
		assert.NoError(t, err, "Unexpected Error")
		assert.True(t, verifyNoSyncSetExists(mocks.fakeKubeClient, &SyncSetEntry{}))
		assert.True(t, verifyNoConfigMapExists(mocks.fakeKubeClient))
	})
}

// verifySyncSetExists verifies that a SyncSet exists that matches the supplied expected SyncSetEntry.
func verifySyncSetExists(c client.Client, expected *SyncSetEntry) bool {
	ss := hivev1.SyncSet{}
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
	secretReferences := ss.Spec.SyncSetCommonSpec.Secrets[0].SourceRef.Name
	if secretReferences == "" {
		return false
	}
	return string(secretReferences) == testsecretReferencesName
}

// verifyNoSyncSetExists verifies that there is no SyncSet present that matches the supplied expected SyncSetEntry.
func verifyNoSyncSetExists(c client.Client, expected *SyncSetEntry) bool {
	ssList := &hivev1.SyncSetList{}
	opts := client.ListOptions{Namespace: testNamespace}
	err := c.List(context.TODO(), &opts, ssList)

	if err != nil {
		if errors.IsNotFound(err) {
			// no syncsets are defined, this is OK
			return true
		}
	}

	for _, ss := range ssList.Items {
		if ss.Name != testClusterName+testOtherSyncSetPostfix {
			// too bad, found a syncset associated with this operator
			return false
		}
	}

	// if we got here, it's good.  list was empty or everything passed
	return true
}

func verifyNoConfigMapExists(c client.Client) bool {
	cmList := &corev1.ConfigMapList{}
	opts := client.ListOptions{Namespace: testNamespace}
	err := c.List(context.TODO(), &opts, cmList)

	if err != nil {
		if errors.IsNotFound(err) {
			// no configmaps are defined, this is OK
			return true
		}
	}

	for _, cm := range cmList.Items {
		if strings.HasSuffix(cm.Name, config.ConfigMapPostfix) {
			// too bad, found a configmap associated with this operator
			return false
		}
	}

	// if we got here, it's good.  list was empty or everything passed
	return true
}
