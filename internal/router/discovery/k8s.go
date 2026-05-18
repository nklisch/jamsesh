package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"jamsesh/internal/router/readyz"
	"jamsesh/internal/router/ring"
)

// podPort is the port appended to each pod's IP address when constructing
// the address for probing and proxying. This matches JAMSESH_BIND default.
const podPort = "8443"

// k8sDiscoverer implements Discoverer using k8s client-go informers to watch
// pods backing a named Service.
type k8sDiscoverer struct {
	namespace   string
	serviceName string
	probe       *readyz.Probe
	interval    time.Duration

	// clientFn produces the Kubernetes clientset. Swappable in tests.
	clientFn func() (kubernetes.Interface, error)
}

// Run builds the k8s informer, watches Pod add/update/delete events, and on
// each event (debounced by interval) re-probes all Running pods and publishes
// the healthy subset. Blocks until ctx is cancelled.
func (d *k8sDiscoverer) Run(ctx context.Context, publish func([]ring.Pod)) error {
	client, err := d.buildClient()
	if err != nil {
		return fmt.Errorf("discovery/k8s: build client: %w", err)
	}
	return d.runWithClient(ctx, client, publish)
}

// KubernetesWithClient returns a Discoverer identical to Kubernetes but uses
// the provided clientset instead of in-cluster config. Intended for tests.
func KubernetesWithClient(namespace, serviceName string, client kubernetes.Interface, probe *readyz.Probe, interval time.Duration) Discoverer {
	d := &k8sDiscoverer{
		namespace:   namespace,
		serviceName: serviceName,
		probe:       probe,
		interval:    interval,
	}
	d.clientFn = func() (kubernetes.Interface, error) { return client, nil }
	return d
}

// buildClient returns the kubernetes.Interface to use. When clientFn is set
// (e.g. in tests), it is called instead of in-cluster config.
func (d *k8sDiscoverer) buildClient() (kubernetes.Interface, error) {
	if d.clientFn != nil {
		return d.clientFn()
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

// runWithClient performs the informer loop using the provided clientset.
// Extracted so tests can inject a fake clientset.
func (d *k8sDiscoverer) runWithClient(ctx context.Context, client kubernetes.Interface, publish func([]ring.Pod)) error {
	factory := informers.NewSharedInformerFactoryWithOptions(
		client,
		d.interval, // resync period — keep the local cache fresh even without events
		informers.WithNamespace(d.namespace),
	)

	podInformer := factory.Core().V1().Pods().Informer()

	// trigger is closed / sent on whenever the pod set may have changed.
	trigger := make(chan struct{}, 1)
	notify := func() {
		select {
		case trigger <- struct{}{}:
		default: // already pending
		}
	}

	var mu sync.Mutex
	prev := neverPublished // sentinel: "no previous publish"

	doProbe := func() {
		addrs := d.listRunningAddrs(podInformer)
		healthy := d.probe.Check(ctx, addrs)
		key := joinSorted(healthy)
		mu.Lock()
		changed := key != prev
		if changed {
			prev = key
		}
		mu.Unlock()
		if changed {
			pods := k8sPodsToPods(healthy)
			publish(pods)
		}
	}

	_, err := podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { notify() },
		UpdateFunc: func(oldObj, newObj interface{}) { notify() },
		DeleteFunc: func(obj interface{}) { notify() },
	})
	if err != nil {
		return fmt.Errorf("discovery/k8s: add event handler: %w", err)
	}

	factory.Start(ctx.Done())

	// Wait for the initial list sync before proceeding.
	if !cache.WaitForCacheSync(ctx.Done(), podInformer.HasSynced) {
		return ctx.Err() // ctx cancelled before sync — normal shutdown path
	}

	// Run initial probe pass now that the cache is warm.
	doProbe()

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-trigger:
			doProbe()
		case <-ticker.C:
			doProbe()
		}
	}
}

// listRunningAddrs returns the "<podIP>:8443" address for every Running pod in
// the informer cache whose namespace matches.
func (d *k8sDiscoverer) listRunningAddrs(informer cache.SharedIndexInformer) []string {
	objs := informer.GetStore().List()
	var addrs []string
	for _, obj := range objs {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			continue
		}
		if pod.Namespace != d.namespace {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.Status.PodIP == "" {
			continue
		}
		addrs = append(addrs, pod.Status.PodIP+":"+podPort)
	}
	return addrs
}

// k8sPodsToPods converts healthy "<podIP>:port" address strings to ring.Pod.
// In k8s mode Pod.ID is set to the address (IP:port) which is stable for the
// lifetime of a running pod and unique across pods.
func k8sPodsToPods(addrs []string) []ring.Pod {
	pods := make([]ring.Pod, len(addrs))
	for i, a := range addrs {
		pods[i] = ring.Pod{
			ID:      a,
			Address: a,
		}
	}
	return pods
}
