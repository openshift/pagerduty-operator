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

package utils

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"testing"

	hiveapis "github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateSyncSetApplyMode(t *testing.T) {

	const (
		testNamespace   = "testNamespace"
		testSyncSetName = "testSyncSet"
	)

	assert.Nil(t, hiveapis.AddToScheme(scheme.Scheme))

	tests := []struct {
		name    string
		objects []runtime.Object
		targetApplyMode hivev1.SyncSetResourceApplyMode
	}{
		{
			name: "Test changing SyncSet applymode from Sync to Upsert",
			objects: []runtime.Object{
				testSyncSet(testSyncSetName, testNamespace, hivev1.SyncResourceApplyMode),
			},
			targetApplyMode: hivev1.UpsertResourceApplyMode,
		},
		{
			name: "Test changing SyncSet applymode from Sync to Sync",
			objects: []runtime.Object{
				testSyncSet(testSyncSetName, testNamespace, hivev1.SyncResourceApplyMode),
			},
			targetApplyMode: hivev1.SyncResourceApplyMode,
		},
	}

	var testLog = logf.Log.WithName("TestUpdateSyncSetApplyMode")
	testLogger := testLog.WithValues("Request.Namespace", testNamespace)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewFakeClient(test.objects...)
			err := UpdateSyncSetApplyMode(testSyncSetName, testNamespace, client, test.targetApplyMode, testLogger)
			assert.NoError(t, err, "Unexpected error updating SyncSet: " + test.name)

			ss := hivev1.SyncSet{}
			err = client.Get(context.TODO(),
				types.NamespacedName{Name: testSyncSetName, Namespace: testNamespace},
				&ss)
			assert.NoError(t, err, "Unexpected error checking SyncSet: " + test.name)
			assert.Equal(t, ss.Spec.ResourceApplyMode, test.targetApplyMode, "SyncSet apply modes do not match:" + test.name)
		})
	}
}

func testSyncSet(syncSetName string, syncSetNamespace string, applyMode hivev1.SyncSetResourceApplyMode) *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      syncSetName,
			Namespace: syncSetNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: applyMode,
				ApplyBehavior:     hivev1.ApplySyncSetApplyBehavior,
			},
			ClusterDeploymentRefs: nil,
		},
	}
}

func TestHasAnnotation(t *testing.T) {

	const (
		testNamespace   = "testNamespace"
		testClusterName = "testCluster"
	)

	assert.Nil(t, hiveapis.AddToScheme(scheme.Scheme))

	tests := []struct {
		name    string
		clusterDeployment *hivev1.ClusterDeployment
		objects []runtime.Object
		expectResult bool
		testAnnotationKey string
		testAnnotationValues []string
		testComparator func(string, string) bool
	}{
		{
			name: "Different Key, Different Value, Suffix Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 1": "value 1",
			}),
			expectResult: false,
			testAnnotationKey: "key 2",
			testAnnotationValues: []string{"value 2"},
			testComparator: strings.HasSuffix,
		},
		{
			name: "Same Key, Same Value, Suffix Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 1": "value 1",
			}),
			testAnnotationKey: "key 1",
			testAnnotationValues: []string{"value 1"},
			testComparator: strings.HasSuffix,
			expectResult: true,
		},
		{
			name: "Same Key, Multiple Values, One value matches, Suffix Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 1": "value 1",
			}),
			testAnnotationKey: "key 1",
			testAnnotationValues: []string{"value 2", "value 1"},
			testComparator: strings.HasSuffix,
			expectResult: true,
		},
		{
			name: "Different Key, Same Value, Suffix Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 2": "value 1",
			}),
			testAnnotationKey: "key 1",
			testAnnotationValues: []string{"value 1"},
			testComparator: strings.HasSuffix,
			expectResult: false,
		},
		{
			name: "Same Key, No Values, Suffix Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 1": "value 1",
			}),
			testAnnotationKey: "key 1",
			testAnnotationValues: []string{},
			testComparator: strings.HasSuffix,
			expectResult: true,
		},
		{
			name: "Different Key, No Values, Suffix Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 2": "value 1",
			}),
			testAnnotationKey: "key 1",
			testAnnotationValues: []string{},
			testComparator: strings.HasSuffix,
			expectResult: false,
		},
		{
			name: "Same Key, Value has suffix, Suffix Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 1": "value 1/matchme",
			}),
			testAnnotationKey: "key 1",
			testAnnotationValues: []string{"matchme"},
			testComparator: strings.HasSuffix,
			expectResult: true,
		},
		{
			name: "Same Key, Multiple Values, One value has suffix, Suffix Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 1": "value 1/matchme",
			}),
			testAnnotationKey: "key 1",
			testAnnotationValues: []string{"dontmatch", "matchme"},
			testComparator: strings.HasSuffix,
			expectResult: true,
		},
		{
			name: "Same Key, Multiple Values, One value has match string, Contains Comparator",
			clusterDeployment: testClusterDeployment(testClusterName, testNamespace, map[string]string {
				"key 1": "value 1/blahblahMATCHMEblahblah",
			}),
			testAnnotationKey: "key 1",
			testAnnotationValues: []string{"dontmatch", "MATCHME"},
			testComparator: strings.Contains,
			expectResult: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := HasAnnotation(test.clusterDeployment, test.testAnnotationKey, test.testComparator, test.testAnnotationValues...)
			assert.Equal(t, res, test.expectResult, "unexpected annotation result:" + test.name)
		})
	}
}

// testClusterDeployment returns a fake ClusterDeployment for an installed cluster to use in testing.
func testClusterDeployment(clusterName string, namespace string, annotations map[string]string) *hivev1.ClusterDeployment {
	cd := hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: clusterName,
		},
	}
	return &cd
}
