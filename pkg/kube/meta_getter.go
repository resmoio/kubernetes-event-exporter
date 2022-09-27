package kube

import (
	"strings"

	lru "github.com/hashicorp/golang-lru"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Getter interface {
	Get(reference *v1.ObjectReference) (metav1.Object, error)
}

type metaGetter struct {
	dynClient dynamic.Interface
	clientset *kubernetes.Clientset
}

func NewMetaGetter(config *rest.Config) Getter {
	return &metaGetter{
		dynClient: dynamic.NewForConfigOrDie(config),
		clientset: kubernetes.NewForConfigOrDie(config),
	}
}

func (g *metaGetter) Get(ref *v1.ObjectReference) (metav1.Object, error) {
	obj, err := GetObject(ref, g.clientset, g.dynClient)
	if err != nil {
		return nil, err
	}
	anno := obj.GetAnnotations()
	for k := range anno {
		if strings.Contains(k, "kubernetes.io/") || strings.Contains(k, "k8s.io/") {
			delete(anno, k)
		}
	}
	obj.SetAnnotations(anno)
	return meta.Accessor(obj)
}

type cacheGetter struct {
	Getter
	cache *lru.ARCCache
}

func NewMetaCacheGetter(config *rest.Config) Getter {
	cache, err := lru.NewARC(1024)
	if err != nil {
		panic("cannot init cache: " + err.Error())
	}
	return &cacheGetter{
		Getter: &metaGetter{
			dynClient: dynamic.NewForConfigOrDie(config),
			clientset: kubernetes.NewForConfigOrDie(config),
		},
		cache: cache,
	}
}

func (g *cacheGetter) Get(ref *v1.ObjectReference) (metav1.Object, error) {
	if val, ok := g.cache.Get(ref.UID); ok {
		return val.(metav1.Object), nil
	}
	obj, err := g.Getter.Get(ref)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	g.cache.Add(obj.GetUID(), obj)
	return obj, nil
}
