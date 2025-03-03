// Copyright 2023 Upbound Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clients

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/upbound/xgql/internal/graph/extensions/live_query"
)

// WithLiveQueries wraps NewCacheFn with a cache.Cache that tracks objects and lists
// and notifies the live query in request context of changes.
func WithLiveQueries(fn NewCacheFn) NewCacheFn {
	return func(cfg *rest.Config, o cache.Options) (cache.Cache, error) {
		c, err := fn(cfg, o)
		if err != nil {
			return nil, err
		}
		return &liveQueryCache{
			Cache:  c,
			scheme: o.Scheme,
		}, nil
	}
}

func isSameObject(a, b client.Object) bool {
	return a.GetName() == b.GetName() && a.GetNamespace() == b.GetNamespace()
}

type liveQueryCache struct {
	cache.Cache
	scheme *kruntime.Scheme
}

func (c *liveQueryCache) trackObject(ctx context.Context, co client.Object) error {
	if !live_query.IsLive(ctx) {
		return nil
	}
	i, err := c.Cache.GetInformer(ctx, co)
	if err != nil {
		return err
	}
	var r toolscache.ResourceEventHandlerRegistration
	r, err = i.AddEventHandler(toolscache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			// If the context is done, remove the handler.
			if !live_query.IsLive(ctx) {
				_ = i.RemoveEventHandler(r)
				return false
			}
			o, ok := obj.(client.Object)
			if !ok {
				return false
			}
			return isSameObject(co, o)
		},
		Handler: toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				live_query.NotifyChanged(ctx)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				live_query.NotifyChanged(ctx)
			},
			DeleteFunc: func(obj interface{}) {
				live_query.NotifyChanged(ctx)
			},
		},
	})
	return err
}

func (c *liveQueryCache) getInformerForListObject(ctx context.Context, list client.ObjectList) (cache.Informer, error) {
	gvk, err := apiutil.GVKForObject(list, c.scheme)
	if err != nil {
		return nil, err
	}

	// We need the non-list GVK, so chop off the "List" from the end of the kind.
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")

	// Handle unstructured.UnstructuredList.
	if _, isUnstructured := list.(kruntime.Unstructured); isUnstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		return c.Cache.GetInformer(ctx, u)
	}
	// Handle metav1.PartialObjectMetadataList.
	if _, isPartialObjectMetadata := list.(*metav1.PartialObjectMetadataList); isPartialObjectMetadata {
		pom := &metav1.PartialObjectMetadata{}
		pom.SetGroupVersionKind(gvk)
		return c.Cache.GetInformer(ctx, pom)
	}

	return c.Cache.GetInformerForKind(ctx, gvk)
}

func (c *liveQueryCache) trackObjectList(ctx context.Context, list client.ObjectList) error {
	if !live_query.IsLive(ctx) {
		return nil
	}
	i, err := c.getInformerForListObject(ctx, list)
	if err != nil {
		return err
	}
	var r toolscache.ResourceEventHandlerRegistration
	r, err = i.AddEventHandler(toolscache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			if !live_query.IsLive(ctx) {
				_ = i.RemoveEventHandler(r)
				return false
			}
			return true
		},
		Handler: toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(_ interface{}) {
				live_query.NotifyChanged(ctx)
			},
			UpdateFunc: func(_, _ interface{}) {
				live_query.NotifyChanged(ctx)
			},
			DeleteFunc: func(_ interface{}) {
				live_query.NotifyChanged(ctx)
			},
		},
	})
	return err
}

// Get implements cache.Cache. It wraps an underlying cache.Cache and sets up an Informer
// event handler that marks current live query as dirty if the current context has a live query.
func (c *liveQueryCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.Cache.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	return c.trackObject(ctx, obj)
}

// List implements cache.Cache. It wraps an underlying cache.Cache and sets up an Informer
// event handler that marks current live query as dirty if the current context has a live query.
func (c *liveQueryCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := c.Cache.List(ctx, list, opts...); err != nil {
		return err
	}
	return c.trackObjectList(ctx, list)
}
