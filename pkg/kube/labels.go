package kube

import (
	lru "github.com/hashicorp/golang-lru"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sync"
)

type LabelCache struct {
	dynClient dynamic.Interface
	clientset *kubernetes.Clientset

	cache *lru.ARCCache
	sync.RWMutex
}

func NewLabelCache(kubeconfig *rest.Config) (*LabelCache) {
	cache, err := lru.NewARC(1024)
	if err != nil {
		panic("cannot init cache: " + err.Error())
	}
	return &LabelCache{
		dynClient: dynamic.NewForConfigOrDie(kubeconfig),
		clientset: kubernetes.NewForConfigOrDie(kubeconfig),
		cache:     cache,
	}
}

func (l *LabelCache) GetLabelsWithCache(uid interface{}, reference *v1.ObjectReference) (map[string]string, error) {
	cacheCheck.Inc()
	if val, ok := l.cache.Get(uid); ok {
		cacheHit.Inc()
		return val.(map[string]string), nil
	}
	cacheMiss.Inc()

	obj, err := GetObject(reference, l.clientset, l.dynClient)
	if err == nil {
		labels := obj.GetLabels()
		l.cache.Add(uid, labels)
		return labels, nil
	}

	if errors.IsNotFound(err) {
		cacheError.Inc()
		// There can be events without the involved objects existing, they seem to be not garbage collected?
		// Marking it nil so that we can return faster
		var empty map[string]string
		l.cache.Add(uid, empty)
		return nil, nil
	}

	// An non-ignorable error occurred
	return nil, err
}
