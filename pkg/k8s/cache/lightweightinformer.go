package cache

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// Source: https://github.com/lyft/clutch/blob/aafa54b2641d1d56d6e909e2140e4b7c490fd181/backend/service/k8s/lightweightinformer.go

// NewLightweightInformer is an informer thats optimized for memory usage with drawbacks.
//
// The reduction in memory consumption does come at a cost, to achieve this we store small objects
// in the informers cache store. We do this by utilizing storing `metav1.PartialObjectMetadata` instead
// of the full Kubernetes object.
// `metav1.PartialObjectMetadata` has just enough metadata for the cache store and DeltaFIFO components to operate normally.
//
// Also to note the memory footprint of the cache store is only part of the story.
// While the informers controller is receiving Kubernetes objects it stores that full object in the DeltaFIFO queue.
// This queue while processed quickly does store a vast amount of objects at any given time and contributes to memory usage greatly.
//
// Drawbacks
// - Update resource event handler does not function as expected, old objects will always return nil.
//   This is because we dont cache the full k8s object to compute deltas.
func NewLightweightInformer(
	lw cache.ListerWatcher,
	objType runtime.Object,
	resync time.Duration,
	h cache.ResourceEventHandler,
) cache.Controller {
	cacheStore := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{})
	fifo := cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{
		KnownObjects:          cacheStore,
		EmitDeltaTypeReplaced: true,
	})

	return cache.New(&cache.Config{
		Queue:            fifo,
		ListerWatcher:    lw,
		ObjectType:       objType,
		FullResyncPeriod: resync,
		RetryOnError:     false,

		Process: func(obj interface{}) error {
			for _, d := range obj.(cache.Deltas) {
				m, err := meta.Accessor(d.Object)
				if err != nil {
					return err
				}

				partial := meta.AsPartialObjectMetadata(m)
				partial.GetObjectKind().SetGroupVersionKind(metav1beta1.SchemeGroupVersion.WithKind("PartialObjectMetadata"))

				switch d.Type {
				case cache.Sync, cache.Replaced, cache.Added, cache.Updated:
					if _, exists, err := cacheStore.Get(partial); err == nil && exists {
						if err := cacheStore.Update(partial); err != nil {
							return err
						}
						h.OnUpdate(nil, d.Object)
					} else {
						if err := cacheStore.Add(partial); err != nil {
							return err
						}
						h.OnAdd(d.Object)
					}
				case cache.Deleted:
					if err := cacheStore.Delete(partial); err != nil {
						return err
					}
					h.OnDelete(d.Object)
				default:
					return fmt.Errorf("Cache type not supported: %s", d.Type)
				}
			}

			return nil
		},
	})
}
