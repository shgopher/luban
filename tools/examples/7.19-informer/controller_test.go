package main

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func newTestController() (*Controller, informers.SharedInformerFactory) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	client := newFakeClient(cm)
	factory := informers.NewSharedInformerFactory(client, 0)
	return NewController(client, factory), factory
}

// 证明 Run 在 WaitForCacheSync 之前不会处理任何对象。
func TestRunWaitsForCacheSync(t *testing.T) {
	c, factory := newTestController()
	stop := make(chan struct{})
	defer close(stop)
	go func() { _ = c.Run(1, stop) }()

	time.Sleep(100 * time.Millisecond)
	if n := c.Attempts("default/demo"); n != 0 {
		t.Fatalf("cache 未同步就开始处理，attempts=%d", n)
	}

	go factory.Start(stop)
	waitFor(t, "reconcile after cache sync", func() bool { return c.Attempts("default/demo") >= 2 })
}

// 证明失败走 AddRateLimited（NumRequeues 增长），成功后 Forget（归零、队列清空）。
func TestRateLimitAndForget(t *testing.T) {
	c, factory := newTestController()
	stop := make(chan struct{})
	defer close(stop)
	go factory.Start(stop)
	go func() { _ = c.Run(1, stop) }()

	key := "default/demo"
	waitFor(t, "first failure", func() bool { return c.Attempts(key) >= 1 })
	select {
	case n := <-c.Requeued():
		if n < 1 {
			t.Fatalf("AddRateLimited 后 NumRequeues=%d, want >=1", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("没有观察到 AddRateLimited")
	}

	waitFor(t, "second reconcile", func() bool { return c.Attempts(key) >= 2 })
	waitFor(t, "Forget resets requeues", func() bool {
		return c.Queue().NumRequeues(key) == 0 && c.Queue().Len() == 0
	})

	if got := c.Attempts(key); got != 2 {
		t.Fatalf("attempts=%d, want 2", got)
	}
}
