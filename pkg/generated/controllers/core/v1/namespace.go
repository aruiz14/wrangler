/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by main. DO NOT EDIT.

package v1

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/wrangler/v2/pkg/apply"
	"github.com/rancher/wrangler/v2/pkg/condition"
	"github.com/rancher/wrangler/v2/pkg/generic"
	"github.com/rancher/wrangler/v2/pkg/kv"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NamespaceController interface for managing Namespace resources.
type NamespaceController interface {
	generic.NonNamespacedControllerInterface[*v1.Namespace, *v1.NamespaceList]
}

// NamespaceClient interface for managing Namespace resources in Kubernetes.
type NamespaceClient interface {
	generic.NonNamespacedClientInterface[*v1.Namespace, *v1.NamespaceList]
}

// NamespaceCache interface for retrieving Namespace resources in memory.
type NamespaceCache interface {
	generic.NonNamespacedCacheInterface[*v1.Namespace]
}

type NamespaceStatusHandler func(obj *v1.Namespace, status v1.NamespaceStatus) (v1.NamespaceStatus, error)

type NamespaceGeneratingHandler func(obj *v1.Namespace, status v1.NamespaceStatus) ([]runtime.Object, v1.NamespaceStatus, error)

func RegisterNamespaceStatusHandler(ctx context.Context, controller NamespaceController, condition condition.Cond, name string, handler NamespaceStatusHandler) {
	statusHandler := &namespaceStatusHandler{
		client:    controller,
		condition: condition,
		handler:   handler,
	}
	controller.AddGenericHandler(ctx, name, generic.FromObjectHandlerToHandler(statusHandler.sync))
}

func RegisterNamespaceGeneratingHandler(ctx context.Context, controller NamespaceController, apply apply.Apply,
	condition condition.Cond, name string, handler NamespaceGeneratingHandler, opts *generic.GeneratingHandlerOptions) {
	statusHandler := &namespaceGeneratingHandler{
		NamespaceGeneratingHandler: handler,
		apply:                      apply,
		name:                       name,
		gvk:                        controller.GroupVersionKind(),
	}
	if opts != nil {
		statusHandler.opts = *opts
	}
	controller.OnChange(ctx, name, statusHandler.Remove)
	RegisterNamespaceStatusHandler(ctx, controller, condition, name, statusHandler.Handle)
}

type namespaceStatusHandler struct {
	client    NamespaceClient
	condition condition.Cond
	handler   NamespaceStatusHandler
}

func (a *namespaceStatusHandler) sync(key string, obj *v1.Namespace) (*v1.Namespace, error) {
	if obj == nil {
		return obj, nil
	}

	origStatus := obj.Status.DeepCopy()
	obj = obj.DeepCopy()
	newStatus, err := a.handler(obj, obj.Status)
	if err != nil {
		// Revert to old status on error
		newStatus = *origStatus.DeepCopy()
	}

	if a.condition != "" {
		if errors.IsConflict(err) {
			a.condition.SetError(&newStatus, "", nil)
		} else {
			a.condition.SetError(&newStatus, "", err)
		}
	}
	if !equality.Semantic.DeepEqual(origStatus, &newStatus) {
		if a.condition != "" {
			// Since status has changed, update the lastUpdatedTime
			a.condition.LastUpdated(&newStatus, time.Now().UTC().Format(time.RFC3339))
		}

		var newErr error
		obj.Status = newStatus
		newObj, newErr := a.client.UpdateStatus(obj)
		if err == nil {
			err = newErr
		}
		if newErr == nil {
			obj = newObj
		}
	}
	return obj, err
}

type namespaceGeneratingHandler struct {
	NamespaceGeneratingHandler
	apply apply.Apply
	opts  generic.GeneratingHandlerOptions
	gvk   schema.GroupVersionKind
	name  string
	seen  sync.Map
}

func (a *namespaceGeneratingHandler) Remove(key string, obj *v1.Namespace) (*v1.Namespace, error) {
	if obj != nil {
		return obj, nil
	}

	obj = &v1.Namespace{}
	obj.Namespace, obj.Name = kv.RSplit(key, "/")
	obj.SetGroupVersionKind(a.gvk)

	if a.opts.UniqueApplyForResourceVersion {
		a.seen.Delete(key)
	}

	return nil, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects()
}

func (a *namespaceGeneratingHandler) Handle(obj *v1.Namespace, status v1.NamespaceStatus) (v1.NamespaceStatus, error) {
	if !obj.DeletionTimestamp.IsZero() {
		return status, nil
	}

	objs, newStatus, err := a.NamespaceGeneratingHandler(obj, status)
	if err != nil || !a.isNewResourceVersion(obj) {
		return newStatus, err
	}

	err = generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects(objs...)
	if err == nil {
		a.seenResourceVersion(obj)
	}
	return newStatus, err
}

func (a *namespaceGeneratingHandler) isNewResourceVersion(obj *v1.Namespace) bool {
	if !a.opts.UniqueApplyForResourceVersion {
		return true
	}

	// Apply once per resource version
	key := obj.Namespace + "/" + obj.Name
	previous, ok := a.seen.Load(key)
	return !ok || previous != obj.ResourceVersion
}

func (a *namespaceGeneratingHandler) seenResourceVersion(obj *v1.Namespace) {
	if !a.opts.UniqueApplyForResourceVersion {
		return
	}

	key := obj.Namespace + "/" + obj.Name
	a.seen.Store(key, obj.ResourceVersion)
}
