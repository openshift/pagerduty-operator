// Copyright 2020 Red Hat
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pagerdutyintegration

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	hiveapis "github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/pagerduty-operator/config"
	pagerdutyapis "github.com/openshift/pagerduty-operator/pkg/apis"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	"github.com/openshift/pagerduty-operator/pkg/kube"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
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
	testPagerDutyIntegrationName    = "testPagerDutyIntegration"
	testClusterName                 = "testCluster"
	testNamespace                   = "testNamespace"
	testIntegrationID               = "ABC123"
	testServiceID                   = "DEF456"
	testAPIKey                      = "test-pd-api-key"
	testEscalationPolicy            = "test-escalation-policy"
	testResolveTimeout              = 300
	testAcknowledgeTimeout          = 300
	testOtherSyncSetPostfix         = "-something-else"
	testsecretReferencesName        = "pd-secret"
	testServicePrefix               = "test-service-prefix"
	fakeClusterDeploymentAnnotation = "managed.openshift.com/fake"
)

type SyncSetEntry struct {
	name                     string
	clusterDeploymentRefName string
	targetSecret             hivev1.SecretReference
}

type SecretEntry struct {
	name         string
	pagerdutyKey string
}

type ClusterDeploymentEntry struct {
	name string
}

type mocks struct {
	fakeKubeClient client.Client
	mockCtrl       *gomock.Controller
	mockPDClient   *pd.MockClient
}

func setupDefaultMocks(t *testing.T, localObjects []runtime.Object) *mocks {
	mocks := &mocks{
		fakeKubeClient: fakekubeclient.NewFakeClient(localObjects...),
		mockCtrl:       gomock.NewController(t),
	}

	mocks.mockPDClient = pd.NewMockClient(mocks.mockCtrl)

	return mocks
}

// testPDISecret creates a fake secret containing pagerduty config details to use for testing.
func testPDISecret() *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: config.OperatorNamespace,
			Name:      config.PagerDutyAPISecretName,
		},
		Data: map[string][]byte{
			config.PagerDutyAPISecretKey: []byte(testAPIKey),
		},
	}
	return s
}

// testCDConfigMap returns a fake configmap for a deployed cluster for testing.
func testCDConfigMap(isHibernating bool, hasLimitedSupport bool) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      config.Name(testServicePrefix, testClusterName, config.ConfigMapSuffix),
		},
		Data: map[string]string{
			"INTEGRATION_ID":       testIntegrationID,
			"SERVICE_ID":           testServiceID,
			"ESCALATION_POLICY_ID": testEscalationPolicy,
			"HIBERNATING":          strconv.FormatBool(isHibernating),
			"LIMITED_SUPPORT":      strconv.FormatBool(hasLimitedSupport),
		},
	}
	return cm
}

// testCDConfigMap returns a fake configmap without ESCALATION_POLICY_ID key for a deployed cluster for testing.
func testCDConfigMapWithoutEscalationPolicy(isHibernating bool, hasLimitedSupport bool) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      config.Name(testServicePrefix, testClusterName, config.ConfigMapSuffix),
		},
		Data: map[string]string{
			"INTEGRATION_ID":  testIntegrationID,
			"SERVICE_ID":      testServiceID,
			"HIBERNATING":     strconv.FormatBool(isHibernating),
			"LIMITED_SUPPORT": strconv.FormatBool(hasLimitedSupport),
		},
	}
	return cm
}

// testCDSecret returns a Secret that will go in the SyncSet for a deployed cluster to use in testing.
func testCDSecret() *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testServicePrefix + "-" + testClusterName + "-" + testsecretReferencesName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			config.PagerDutySecretKey: []byte(testIntegrationID),
		},
	}
	return s
}

// testCDSyncSet returns a SyncSet for an existing testClusterDeployment to use in testing.
func testCDSyncSet() *hivev1.SyncSet {
	secretName := config.Name(testServicePrefix, testClusterName, config.SecretSuffix)
	secret := kube.GeneratePdSecret(testNamespace, secretName, testIntegrationID)
	pdi := testPagerDutyIntegration()
	ss := kube.GenerateSyncSet(testNamespace, testClusterName, secret, pdi)
	return ss
}

func testPagerDutyIntegration() *pagerdutyv1alpha1.PagerDutyIntegration {
	return &pagerdutyv1alpha1.PagerDutyIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPagerDutyIntegrationName,
			Namespace: config.OperatorNamespace,
		},
		Spec: pagerdutyv1alpha1.PagerDutyIntegrationSpec{
			AcknowledgeTimeout: testAcknowledgeTimeout,
			ResolveTimeout:     testResolveTimeout,
			EscalationPolicy:   testEscalationPolicy,
			ServicePrefix:      testServicePrefix,
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{config.ClusterDeploymentManagedLabel: "true"},
			},
			PagerdutyApiKeySecretRef: corev1.SecretReference{
				Name:      config.PagerDutyAPISecretName,
				Namespace: config.OperatorNamespace,
			},
			TargetSecretRef: corev1.SecretReference{
				Name:      config.Name(testServicePrefix, testClusterName, config.SecretSuffix),
				Namespace: testNamespace,
			},
		},
	}
}

func updatedTestPagerDutyIntegration() *pagerdutyv1alpha1.PagerDutyIntegration {
	testPDI := testPagerDutyIntegration()
	testPDI.Spec.EscalationPolicy = "new-escalation-policy"
	return testPDI
}

// testClusterDeployment returns a fake ClusterDeployment for an installed cluster to use in testing.
func testClusterDeployment(isInstalled bool, isManaged bool, hasFinalizer bool, isDeleting bool, isHibernating bool, isFake bool, isLimitedSupport bool) *hivev1.ClusterDeployment {
	labelMap := map[string]string{
		config.ClusterDeploymentManagedLabel:        strconv.FormatBool(isManaged),
		config.ClusterDeploymentLimitedSupportLabel: strconv.FormatBool(isLimitedSupport),
	}
	annotationMap := map[string]string{fakeClusterDeploymentAnnotation: strconv.FormatBool(isFake)}
	cd := hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testClusterName,
			Namespace:   testNamespace,
			Labels:      labelMap,
			Annotations: annotationMap,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testClusterName,
		},
	}
	cd.Spec.Installed = isInstalled

	if isDeleting {
		now := metav1.Now()
		cd.DeletionTimestamp = &now
	}

	if hasFinalizer {
		cd.SetFinalizers([]string{config.PagerDutyFinalizerPrefix + testPagerDutyIntegrationName})
	}

	if isHibernating {
		cd.Spec.PowerState = hivev1.HibernatingClusterPowerState
		cd.Status.Conditions = []hivev1.ClusterDeploymentCondition{
			{
				Type:   hivev1.ClusterHibernatingCondition,
				Status: corev1.ConditionTrue,
				Reason: hivev1.HibernatingHibernationReason,
			},
		}
	} else {
		cd.Status.Conditions = []hivev1.ClusterDeploymentCondition{
			{
				Type:   hivev1.ClusterHibernatingCondition,
				Status: corev1.ConditionFalse,
				Reason: hivev1.RunningReadyReason,
			},
		}
	}

	return &cd
}

func TestReconcilePagerDutyIntegration(t *testing.T) {
	assert.Nil(t, hiveapis.AddToScheme(scheme.Scheme))
	assert.Nil(t, pagerdutyapis.AddToScheme(scheme.Scheme))

	// expectedSyncSet is used by tests that _expect_ a SS
	expectedSyncSet := &SyncSetEntry{
		name:                     config.Name(testServicePrefix, testClusterName, config.SecretSuffix),
		clusterDeploymentRefName: testClusterName,
		targetSecret: hivev1.SecretReference{
			Name:      testPagerDutyIntegration().Spec.TargetSecretRef.Name,
			Namespace: testPagerDutyIntegration().Spec.TargetSecretRef.Namespace,
		},
	}

	// expectedSecret is used by test that _expect_ a Secret
	expectedSecret := &SecretEntry{
		name:         config.Name(testServicePrefix, testClusterName, config.SecretSuffix),
		pagerdutyKey: testIntegrationID,
	}

	// expectedClusterDeployment is used by tests to lookup finalizer (there is always a CD)
	expectedClusterDeployment := &ClusterDeploymentEntry{
		name: testClusterName,
	}

	// EVERY test needs: CD, PDI Secret, PDI
	tests := []struct {
		name                    string
		localObjects            []runtime.Object
		expectPDSetup           bool
		verifyClusterDeployment func(client.Client, *ClusterDeploymentEntry) bool
		setupPDMock             func(*pd.MockClientMockRecorder)
	}{
		{
			name: "Test Not Installed",
			localObjects: []runtime.Object{
				testClusterDeployment(false, false, false, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Fake Cluster",
			localObjects: []runtime.Object{
				testClusterDeployment(true, false, false, false, false, true, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, No Finalizer, Not Deleting, PD Not Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, false, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(1).DoAndReturn(
					func(data *pd.Data) (string, error) {
						data.ServiceID = "XYZ123"
						data.IntegrationID = "LMN456"
						data.EscalationPolicyID = testEscalationPolicy
						return data.IntegrationID, nil
					})
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(1)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
				r.DisableService(gomock.Any()).Return(nil).Times(0)
				r.EnableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Not Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(1).DoAndReturn(
					func(data *pd.Data) (string, error) {
						data.ServiceID = "XYZ123"
						data.IntegrationID = "LMN456"
						data.EscalationPolicyID = testEscalationPolicy
						return data.IntegrationID, nil
					})
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(1)
				r.UpdateEscalationPolicy(gomock.Any()).Return(nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
				r.DisableService(gomock.Any()).Return(nil).Times(0)
				r.EnableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup, Missing CM",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(1).DoAndReturn(
					func(data *pd.Data) (string, error) {
						data.ServiceID = "XYZ123"
						data.IntegrationID = "LMN456"
						data.EscalationPolicyID = testEscalationPolicy
						return data.IntegrationID, nil
					}) // unit test not support "lookup"
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0) // secret already exists, won't recreate
				r.UpdateEscalationPolicy(gomock.Any()).Return(nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
				r.DisableService(gomock.Any()).Return(nil).Times(0)
				r.EnableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup, Missing SyncSet",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
				r.DisableService(gomock.Any()).Return(nil).Times(0)
				r.EnableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup, Missing Secret",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
				r.DisableService(gomock.Any()).Return(nil).Times(0)
				r.EnableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Deleting, PD Not Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, true, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Deleting, PD Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, true, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetService(gomock.Any()).Return(nil, nil).Times(1)
				r.DeleteService(gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "Test Managed, No Finalizer, Deleting, PD Not Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, false, true, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Not Managed, No Finalizer, Not Deleting, PD Not Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, false, false, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Not Managed, Finalizer, Not Deleting, PD Not Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, false, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Not Managed, Finalizer, Not Deleting, PD Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, false, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetService(gomock.Any()).Return(nil, nil).Times(1)
				r.DeleteService(gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "Test Not Managed, Finalizer, Not Deleting, PD Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, false, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetService(gomock.Any()).Return(nil, nil).Times(1)
				r.DeleteService(gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "Test Not Managed, Finalizer, Deleting, PD Not Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, false, true, true, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Not Managed, Finalizer, Deleting, PD Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, false, true, true, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetService(gomock.Any()).Return(nil, nil).Times(1)
				r.DeleteService(gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "Test Not Managed, No Finalizer, Deleting, PD Not Setup",
			localObjects: []runtime.Object{
				testClusterDeployment(true, false, false, true, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: false,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DeleteService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Not Setup, Hibernating",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, true, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(1).DoAndReturn(
					func(data *pd.Data) (string, error) {
						data.ServiceID = "XYZ123"
						data.IntegrationID = "LMN456"
						data.EscalationPolicyID = testEscalationPolicy
						return data.IntegrationID, nil
					})
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(1)
				r.UpdateEscalationPolicy(gomock.Any()).Return(nil).Times(0)
				r.DisableService(gomock.Any()).Return(nil).Times(1)
				r.EnableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup and Hibernating, transition to Active",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(true, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.EnableService(gomock.Any()).Return(nil).Times(1)
				r.DisableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup and Active, transition to Hibernating",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, true, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DisableService(gomock.Any()).Return(nil).Times(1)
				r.EnableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup, Not Hibernating, Limited Support",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, true),
				testPDISecret(),
				testCDSecret(),
				testCDSyncSet(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, false),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.DisableService(gomock.Any()).Return(nil).Times(1)
				r.EnableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup, Not Hibernating, Not in Limited Support",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				testCDSecret(),
				testCDSyncSet(),
				testPagerDutyIntegration(),
				testCDConfigMap(false, true),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.EnableService(gomock.Any()).Return(nil).Times(1)
				r.DisableService(gomock.Any()).Return(nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup, Not Hibernating, Not in Limited Support, CM PDI escalation policy missing",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				testPagerDutyIntegration(),
				testCDConfigMapWithoutEscalationPolicy(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.UpdateEscalationPolicy(gomock.Any()).Return(nil).Times(0)
				r.GetService(gomock.Any()).Return(nil, nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup, Not Hibernating/Limited Support, CM PDI escalation policy exists, PDI escalation policy changed",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				updatedTestPagerDutyIntegration(),
				testCDConfigMap(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.UpdateEscalationPolicy(gomock.Any()).Return(nil).Times(1)
				r.GetService(gomock.Any()).Return(nil, nil).Times(0)
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
			},
		},
		{
			name: "Test Managed, Finalizer, Not Deleting, PD Setup, Not Hibernating/Limited Support, CM PDI escalation policy missing, PDI escalation policy changed",
			localObjects: []runtime.Object{
				testClusterDeployment(true, true, true, false, false, false, false),
				testPDISecret(),
				updatedTestPagerDutyIntegration(),
				testCDConfigMapWithoutEscalationPolicy(false, false),
				testCDSyncSet(),
				testCDSecret(),
			},
			expectPDSetup: true,
			setupPDMock: func(r *pd.MockClientMockRecorder) {
				r.UpdateEscalationPolicy(gomock.Any()).Return(nil).Times(0)
				r.GetService(gomock.Any()).Return(nil, nil).Times(0)
				r.CreateService(gomock.Any()).Return(testIntegrationID, nil).Times(0)
				r.GetIntegrationKey(gomock.Any()).Return(testIntegrationID, nil).Times(0)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t, test.localObjects)
			test.setupPDMock(mocks.mockPDClient.EXPECT())

			defer mocks.mockCtrl.Finish()

			rpdi := &ReconcilePagerDutyIntegration{
				client:   mocks.fakeKubeClient,
				scheme:   scheme.Scheme,
				pdclient: func(s1 string, s2 string) pd.Client { return mocks.mockPDClient },
			}

			// 1st run sets finalizer
			_, err1 := rpdi.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testPagerDutyIntegrationName,
					Namespace: config.OperatorNamespace,
				},
			})

			// 2nd run does the initial work
			_, err2 := rpdi.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testPagerDutyIntegrationName,
					Namespace: config.OperatorNamespace,
				},
			})

			// 3rd run should be a noop, we need to confirm
			_, err3 := rpdi.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testPagerDutyIntegrationName,
					Namespace: config.OperatorNamespace,
				},
			})

			// Assert
			assert.Nil(t, err1, "Unexpected Error with Reconcile (1 of 3)")
			assert.Nil(t, err2, "Unexpected Error with Reconcile (2 of 3)")
			assert.Nil(t, err3, "Unexpected Error with Reconcile (3 of 3)")
			if test.expectPDSetup {
				// should see a syncset, secret, configmap, and finalizer on CD
				assert.True(t, verifySyncSetExists(mocks.fakeKubeClient, expectedSyncSet), "verifySyncSets: "+test.name)
				assert.True(t, verifySecretExists(mocks.fakeKubeClient, expectedSecret), "verifySecretExists: "+test.name)
				assert.True(t, verifyFinalizer(mocks.fakeKubeClient, expectedClusterDeployment), "verifyFinalizer: "+test.name)
				assert.True(t, verifyConfigMapExists(mocks.fakeKubeClient), "verifyConfigMapExists: "+test.name)
			} else {
				// expect no syncset, secret, configmap, OR finalizer on CD
				assert.True(t, verifyNoSyncSetExists(mocks.fakeKubeClient), "verifyNoSyncSetExists: "+test.name)
				assert.True(t, verifyNoSecretExists(mocks.fakeKubeClient), "verifyNoSecretExists: "+test.name)
				assert.True(t, verifyNoFinalizer(mocks.fakeKubeClient, expectedClusterDeployment), "verifyNoFinalizer: "+test.name)
				assert.True(t, verifyNoConfigMapExists(mocks.fakeKubeClient), "verifyNoConfigMapExists: "+test.name)
			}
		})
	}
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
	return string(secretReferences) == expected.name
}

// verifyNoSyncSetExists verifies that there is no SyncSet present that matches the supplied expected SyncSetEntry.
func verifyNoSyncSetExists(c client.Client) bool {
	ssList := &hivev1.SyncSetList{}
	opts := client.ListOptions{Namespace: testNamespace}
	err := c.List(context.TODO(), ssList, &opts)

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

// verifyFinalizer verifies that there is no PD finalizer on the CD.
func verifyFinalizer(c client.Client, expected *ClusterDeploymentEntry) bool {
	cd := hivev1.ClusterDeployment{}
	err := c.Get(context.TODO(),
		types.NamespacedName{Name: expected.name, Namespace: testNamespace}, &cd)
	if err != nil {
		return false
	}

	if expected.name != cd.Name {
		return false
	}

	clusterDeploymentFinalizerName := config.PagerDutyFinalizerPrefix + testPagerDutyIntegrationName

	for _, finalizer := range cd.GetObjectMeta().GetFinalizers() {
		if finalizer == clusterDeploymentFinalizerName {
			// we found a matching finalizer, this is a success
			return true
		}
	}

	// if we got here, it's bad.  no matching finalizers
	return false
}

// verifyNoFinalizer verifies that there is no PD finalizer on the CD.
func verifyNoFinalizer(c client.Client, expected *ClusterDeploymentEntry) bool {
	cd := hivev1.ClusterDeployment{}
	err := c.Get(context.TODO(),
		types.NamespacedName{Name: expected.name, Namespace: testNamespace}, &cd)
	if err != nil {
		return false
	}

	if expected.name != cd.Name {
		return false
	}

	for _, finalizer := range cd.GetObjectMeta().GetFinalizers() {
		if strings.HasPrefix(finalizer, config.PagerDutyFinalizerPrefix) {
			// we found a matching finalizer, this is a failure
			return false
		}
	}

	// if we got here, it's good.  no matching finalizers
	return true
}

// verifySecretExists verifies that the secret which referenced by the SyncSet exists in the test namespace
func verifySecretExists(c client.Client, expected *SecretEntry) bool {
	secret := corev1.Secret{}
	err := c.Get(context.TODO(),
		types.NamespacedName{Name: expected.name, Namespace: testNamespace},
		&secret)

	if err != nil {
		return false
	}

	if expected.name != secret.Name {
		return false
	}

	if expected.pagerdutyKey != string(secret.Data["PAGERDUTY_KEY"]) {
		return false
	}

	return true
}

// verifyNoSecretExists verifies that the secret which referred by SyncSet does not exist
func verifyNoSecretExists(c client.Client) bool {
	secretList := &corev1.SecretList{}
	opts := client.ListOptions{Namespace: testNamespace}
	err := c.List(context.TODO(), secretList, &opts)

	if err != nil {
		if errors.IsNotFound(err) {
			return true
		}
	}

	for _, secret := range secretList.Items {
		if secret.Name == testsecretReferencesName {
			return false
		}
	}

	return true
}

func verifyConfigMapExists(c client.Client) bool {
	cmList := &corev1.ConfigMapList{}
	opts := client.ListOptions{Namespace: testNamespace}
	err := c.List(context.TODO(), cmList, &opts)

	if err != nil {
		if errors.IsNotFound(err) {
			// no configmaps are defined, this is a failure
			return false
		}
	}

	for _, cm := range cmList.Items {
		if strings.HasSuffix(cm.Name, config.ConfigMapSuffix) {
			// found a configmap associated with this operator!
			return true
		}
	}

	// if we got here, it's bad.
	return false
}

func verifyNoConfigMapExists(c client.Client) bool {
	cmList := &corev1.ConfigMapList{}
	opts := client.ListOptions{Namespace: testNamespace}
	err := c.List(context.TODO(), cmList, &opts)

	if err != nil {
		if errors.IsNotFound(err) {
			// no configmaps are defined, this is OK
			return true
		}
	}

	for _, cm := range cmList.Items {
		if strings.HasSuffix(cm.Name, config.ConfigMapSuffix) {
			// too bad, found a configmap associated with this operator
			return false
		}
	}

	// if we got here, it's good.  list was empty or everything passed
	return true
}
