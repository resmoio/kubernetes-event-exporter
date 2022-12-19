package kube

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	cacheCheck = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cacheCheck",
		Help: "The total number of times the cache was checked",
	})
	cacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cacheHit",
		Help: "The total number of times the data needed was in the cache",
	})
	cacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cacheMiss",
		Help: "The total number of times the data needed was NOT found in the cache",
	})
	cacheError = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cacheError",
		Help: "The total number of times an error ocurred consulting the cache",
	})
	eventsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "events_sent",
		Help: "The total number of events sent",
	})
	watchErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "watch_errors",
		Help: "The total number of errors received from the informer",
	})
)
