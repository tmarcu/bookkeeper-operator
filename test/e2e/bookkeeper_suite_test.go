package e2e

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pravega/bookkeeper-operator/pkg/apis"
	"github.com/pravega/bookkeeper-operator/pkg/apis/bookkeeper/v1alpha1"
	"github.com/pravega/bookkeeper-operator/pkg/controller/bookkeepercluster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	requests        chan reconcile.Request
	expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "zookeeper", Namespace: "default"}}
	cfg             *rest.Config
	k8sClient       client.Client
	testEnv         *envtest.Environment
)

const timeout = time.Second * 500

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RegisterTestingT(t)
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crds")},
		ErrorIfCRDPathMissing: true,
		Scheme:                scheme.Scheme,
	}
	g := NewGomegaWithT(t)

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(apis.AddToScheme(scheme.Scheme)).Should(Succeed())
	Expect(v1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme)).Should(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).Should(Succeed())
	Expect(appsv1.AddToScheme(scheme.Scheme)).Should(Succeed())

	mgr, err := controllerruntime.NewManager(cfg, manager.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(mgr).NotTo(BeNil())

	var recFn reconcile.Reconciler
	recFn, requests = SetupTestReconcile(bookkeepercluster.NewReconciler(mgr))
	err = bookkeepercluster.Add(mgr, recFn)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	mgrCancelFunc, mgrWaitgroup := StartTestManager(mgr, g)

	defer func() {
		mgrCancelFunc()
		mgrWaitgroup.Wait()
	}()
	RunSpecs(t, "Bookkeeper controller reconcile")
}

var _ = AfterSuite(func() {
	By("tearing down test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// SetupTestReconcile returns a reconcile.Reconcile implementation that delegates to inner and
// writes the request to requests after Reconcile is finished.
func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests = make(chan reconcile.Request)
	fn := reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(context.TODO(), req)
		requests <- req
		return result, err
	})
	return fn, requests
}

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager, g *GomegaWithT) (context.CancelFunc, *sync.WaitGroup) {
	ctx, cancel := context.WithCancel(context.TODO())
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
	return cancel, wg
}