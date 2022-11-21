package kube

import (
	"sync"

	lru "github.com/hashicorp/golang-lru"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type LabelCache struct {
	dynClient dynamic.Interface
	clientset *kubernetes.Clientset

	cache *lru.ARCCache
	sync.RWMutex
}

func NewLabelCache(kubeconfig *rest.Config) *LabelCache {
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

func (l *LabelCache) GetLabelsWithCache(reference *v1.ObjectReference) (map[string]string, error) {
	uid := reference.UID

	if val, ok := l.cache.Get(uid); ok {
		return val.(map[string]string), nil
	}

	obj, err := GetObject(reference, l.clientset, l.dynClient)
	if err == nil {
		labels := obj.GetLabels()
		l.cache.Add(uid, labels)
		return labels, nil
	}

	if errors.IsNotFound(err) {
		// There can be events without the involved objects existing, they seem to be not garbage collected?
		// Marking it nil so that we can return faster
		var empty map[string]string
		l.cache.Add(uid, empty)
		return nil, nil
	}

	// An non-ignorable error occurred
	return nil, err
}

func NewMockLabelCache() *LabelCache {
	cache, _ := lru.NewARC(1024)
	uid := types.UID("test")
	cache.Add(uid, map[string]string{"test": "test"})
	return &LabelCache{
		cache: cache,
	}
}
