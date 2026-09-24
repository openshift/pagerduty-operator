// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /test/e2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e

package osde2etests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	pdiv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Pagerduty Operator", Ordered, Label("Suite: operators"), func() {
	var (
		k8s       *openshift.Client
		dynClient dynamic.Interface
	)
	const (
		namespace      = "pagerduty-operator"
		deploymentName = "pagerduty-operator"
		operatorName   = "pagerduty-operator"
		crdName        = "pagerdutyintegrations.pagerduty.openshift.io"
		testCRName     = "pdi-e2e-test"
		pollingTimeout = 3 * time.Minute
	)

	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")

		Expect(pdiv1alpha1.AddToScheme(k8s.GetScheme())).Should(Succeed(), "unable to register PagerDutyIntegration scheme")

		dynClient, err = dynamic.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "unable to create dynamic client")
	})

	It("is installed", func(ctx context.Context) {
		By("checking the namespace exists")
		err := k8s.Get(ctx, namespace, "", &corev1.Namespace{})
		Expect(err).ShouldNot(HaveOccurred(), "namespace %s not found", namespace)

		By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, k8s, deploymentName, namespace).Should(BeAvailable())
	})

	PIt("can be upgraded", func(ctx context.Context) {
		By("forcing operator upgrade")
		err := k8s.UpgradeOperator(ctx, operatorName, namespace)
		Expect(err).ShouldNot(HaveOccurred(), "operator upgrade failed")
	})

	It("has the CRD installed", func(ctx context.Context) {
		By("checking that the PagerDutyIntegration CRD exists and is established")
		crdGVR := schema.GroupVersionResource{
			Group:    "apiextensions.k8s.io",
			Version:  "v1",
			Resource: "customresourcedefinitions",
		}
		crd, err := dynClient.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred(), "CRD %s not found", crdName)

		conditions, found, err := unstructured.NestedSlice(crd.Object, "status", "conditions")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(found).To(BeTrue(), "CRD status.conditions not found")

		established := false
		for _, c := range conditions {
			cMap, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if cMap["type"] == "Established" && cMap["status"] == "True" {
				established = true
				break
			}
		}
		Expect(established).To(BeTrue(), "CRD %s is not in Established condition", crdName)
	})

	It("can create and accept a PagerDutyIntegration CR", func(ctx context.Context) {
		By("creating a test PagerDutyIntegration CR")
		pdi := &pdiv1alpha1.PagerDutyIntegration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testCRName,
				Namespace: namespace,
			},
			Spec: pdiv1alpha1.PagerDutyIntegrationSpec{
				EscalationPolicy: "PXXXXXX",
				ServicePrefix:    "e2e-test",
				PagerdutyApiKeySecretRef: corev1.SecretReference{
					Name:      "pagerduty-api-key-e2e",
					Namespace: namespace,
				},
				ClusterDeploymentSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"api.openshift.com/e2e-test": "true",
					},
				},
				TargetSecretRef: corev1.SecretReference{
					Name:      "pd-secret-e2e",
					Namespace: "openshift-monitoring",
				},
			},
		}

		err := k8s.Create(ctx, pdi)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create PagerDutyIntegration CR")

		By("verifying the CR was accepted and can be retrieved")
		created := &pdiv1alpha1.PagerDutyIntegration{}
		err = k8s.Get(ctx, testCRName, namespace, created)
		Expect(err).ShouldNot(HaveOccurred(), "failed to get created PagerDutyIntegration CR")
		Expect(created.Spec.EscalationPolicy).To(Equal("PXXXXXX"))
		Expect(created.Spec.ServicePrefix).To(Equal("e2e-test"))

		By("verifying the operator deployment remains healthy after CR creation")
		deployment := &appsv1.Deployment{}
		err = k8s.Get(ctx, deploymentName, namespace, deployment)
		Expect(err).ShouldNot(HaveOccurred(), "operator deployment not found after CR creation")
		Expect(deployment.Status.AvailableReplicas).To(BeNumerically(">=", 1),
			"operator should remain available after CR creation")
	})

	It("cleans up the test CR", func(ctx context.Context) {
		By("deleting the test PagerDutyIntegration CR")
		pdi := &pdiv1alpha1.PagerDutyIntegration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testCRName,
				Namespace: namespace,
			},
		}

		err := k8s.Delete(ctx, pdi)
		if k8serrors.IsNotFound(err) {
			// CR was already cleaned up, nothing to do
			return
		}
		Expect(err).ShouldNot(HaveOccurred(), "failed to delete PagerDutyIntegration CR")

		By("verifying the CR is removed")
		Eventually(func(g Gomega) {
			err := k8s.Get(ctx, testCRName, namespace, &pdiv1alpha1.PagerDutyIntegration{})
			g.Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "CR should be deleted, got error: %v", err)
		}).WithTimeout(pollingTimeout).WithPolling(5 * time.Second).Should(Succeed())
	})

	AfterAll(func(ctx context.Context) {
		By("ensuring test CR cleanup in AfterAll")
		pdi := &pdiv1alpha1.PagerDutyIntegration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testCRName,
				Namespace: namespace,
			},
		}
		err := k8s.Delete(ctx, pdi)
		if err != nil && !k8serrors.IsNotFound(err) {
			GinkgoLogr.Error(err, "failed to clean up test PagerDutyIntegration CR")
		}
	})
})
