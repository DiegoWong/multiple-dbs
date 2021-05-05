/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	dbv1alpha1 "github.com/DiegoWong/multiple-dbs.git/api/v1alpha1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})

}

var _ = Describe("multiple dbs test", func() {
	const (
		MultipleDbName = "test-multipledb"
		Namespace      = "default"

		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)
	Context("creating CR", func() {
		It("Should create CR", func() {
			ctx := context.Background()
			multipledbs := &dbv1alpha1.MultipleDbs{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "db.foo.com/v1alpha1",
					Kind:       "MultipleDbs",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      MultipleDbName,
					Namespace: Namespace,
				},
				Spec: dbv1alpha1.MultipleDbsSpec{
					Dbs: []dbv1alpha1.DbImage{
						{
							Image:    "test",
							Replicas: 1,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, multipledbs)).Should(Succeed())

			multipleDbsLookupKey := types.NamespacedName{Name: MultipleDbName, Namespace: Namespace}
			createdMultipleDbs := &dbv1alpha1.MultipleDbs{}
			// We'll need to retry getting this newly created CronJob, given that creation may not immediately happen.
			k8sClient.Get(ctx, multipleDbsLookupKey, createdMultipleDbs)
			// Let's make sure our Schedule string value was properly converted/handled.
			Expect(createdMultipleDbs.Spec.Dbs[0].Image).Should(Equal("test"))
			By("By creating a new Deployment")
			deployment := &appsv1.Deployment{}
			deploymentLookupKey := types.NamespacedName{Name: "test-multipledb-test", Namespace: Namespace}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentLookupKey, deployment)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(deployment.ObjectMeta.Name).Should(Equal("test-multipledb-test"))
		})
	})
})

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = dbv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})

	err = (&MultipleDbsReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("multipledbs"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
