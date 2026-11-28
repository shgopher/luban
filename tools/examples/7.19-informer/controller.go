package main

import (
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controller 是最小的“Informer + WorkQueue”骨架：
// EventHandler 只入队 key，worker 从缓存重读对象，失败退避、成功 Forget。
type Controller struct {
	client kubernetes.Interface
	lister corelisters.ConfigMapLister
	synced cache.InformerSynced
	queue  workqueue.RateLimitingInterface

	mu       sync.Mutex
	attempts map[string]int
	failedCh chan string
	requeued chan int
}

func NewController(client kubernetes.Interface, factory informers.SharedInformerFactory) *Controller {
	cmInformer := factory.Core().V1().ConfigMaps()
	c := &Controller{
		client:   client,
		lister:   cmInformer.Lister(),
		synced:   cmInformer.Informer().HasSynced,
		queue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "configmaps"),
		attempts: map[string]int{},
		failedCh: make(chan string, 8),
		requeued: make(chan int, 8),
	}
	cmInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueue,
		UpdateFunc: func(_, newObj interface{}) {
			c.enqueue(newObj)
		},
		DeleteFunc: c.enqueue,
	})
	return c
}

func (c *Controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err == nil {
		c.queue.Add(key)
	}
}

// Run 先等缓存同步，再启动 worker；缓存没同步就处理会把“还没看到”误判成“不存在”。
func (c *Controller) Run(workers int, stopCh <-chan struct{}) error {
	defer c.queue.ShutDown()
	if !cache.WaitForCacheSync(stopCh, c.synced) {
		return fmt.Errorf("informer cache 未同步")
	}
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	<-stopCh
	return nil
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	if err := c.syncHandler(key.(string)); err != nil {
		// 失败：指数退避后重新入队；不 Forget，重试计数保留。
		c.queue.AddRateLimited(key)
		select {
		case c.requeued <- c.queue.NumRequeues(key):
		default:
		}
		return true
	}
	// 成功：清掉重试计数，避免下次失败从很高的退避开始。
	c.queue.Forget(key)
	return true
}

func (c *Controller) syncHandler(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil // 非法 key 直接丢弃
	}
	cm, err := c.lister.ConfigMaps(ns).Get(name)
	if apierrors.IsNotFound(err) {
		return nil // 缓存里没有 = 对象已删；真实控制器在这里清理外部资源
	}
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.attempts[key]++
	n := c.attempts[key]
	c.mu.Unlock()

	// 第一次故意失败，用来观察 AddRateLimited；第二次成功，用来观察 Forget。
	if n == 1 {
		select {
		case c.failedCh <- key:
		default:
		}
		return fmt.Errorf("injected transient error for %s", key)
	}
	_ = cm
	return nil
}

func (c *Controller) Attempts(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempts[key]
}

func (c *Controller) Failed() <-chan string { return c.failedCh }

func (c *Controller) Requeued() <-chan int { return c.requeued }

func (c *Controller) Queue() workqueue.RateLimitingInterface { return c.queue }

func newFakeClient(objects ...runtime.Object) kubernetes.Interface {
	return fake.NewSimpleClientset(objects...)
}

func demo() {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	client := newFakeClient(cm)
	factory := informers.NewSharedInformerFactory(client, 0)
	c := NewController(client, factory)
	stop := make(chan struct{})
	go factory.Start(stop)
	go func() { _ = c.Run(1, stop) }()

	key := "default/demo"
	<-c.Failed()
	fmt.Printf("first reconcile failed, requeues=%d\n", c.queue.NumRequeues(key))
	for c.Attempts(key) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Printf("second reconcile ok, attempts=%d requeues=%d queue_len=%d\n",
		c.Attempts(key), c.queue.NumRequeues(key), c.queue.Len())
	close(stop)
}
