package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
)

// realmRefs returns the realmRef / clusterRealmRef pair of a realm-dependent
// resource.
type realmRefs func(client.Object) (*keycloakv1beta1.ResourceRef, *keycloakv1beta1.ClusterResourceRef)

// watchRealms re-enqueues the objects produced by newList when the
// KeycloakRealm or ClusterKeycloakRealm they reference changes readiness, so a
// dependent applied together with its realm converges as soon as the realm is
// Ready instead of waiting for its error requeue.
func watchRealms(b *builder.Builder, c client.Client, newList func() client.ObjectList, refs realmRefs) *builder.Builder {
	return b.
		Watches(
			&keycloakv1beta1.KeycloakRealm{},
			handler.EnqueueRequestsFromMapFunc(findForRealm(c, newList, refs)),
			builder.WithPredicates(realmReadinessChanged()),
		).
		Watches(
			&keycloakv1beta1.ClusterKeycloakRealm{},
			handler.EnqueueRequestsFromMapFunc(findForClusterRealm(c, newList, refs)),
			builder.WithPredicates(realmReadinessChanged()),
		)
}

// findForRealm maps a KeycloakRealm to the dependents in its namespace whose
// realmRef names it.
func findForRealm(c client.Client, newList func() client.ObjectList, refs realmRefs) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		return listMatching(ctx, c, newList(), []client.ListOption{client.InNamespace(obj.GetNamespace())}, func(o client.Object) bool {
			ref, _ := refs(o)
			return ref != nil && ref.Name == obj.GetName()
		})
	}
}

// findForClusterRealm maps a ClusterKeycloakRealm to the dependents in any
// namespace whose clusterRealmRef names it.
func findForClusterRealm(c client.Client, newList func() client.ObjectList, refs realmRefs) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		return listMatching(ctx, c, newList(), nil, func(o client.Object) bool {
			_, ref := refs(o)
			return ref != nil && ref.Name == obj.GetName()
		})
	}
}

func listMatching(ctx context.Context, c client.Client, list client.ObjectList, opts []client.ListOption, matches func(client.Object) bool) []reconcile.Request {
	if err := c.List(ctx, list, opts...); err != nil {
		return nil
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, item := range items {
		obj, ok := item.(client.Object)
		if !ok || !matches(obj) {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			},
		})
	}
	return requests
}

// realmReadinessChanged filters realm events down to the ones that can change
// the outcome of ResolveRealm for a dependent: readiness or resolved realm name
// transitions and deletions. Creates are ignored because a realm is never
// created Ready, and dependents reconcile themselves on their own creation.
func realmReadinessChanged() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldReady, oldName := realmReadiness(e.ObjectOld)
			newReady, newName := realmReadiness(e.ObjectNew)
			return oldReady != newReady || oldName != newName
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func realmReadiness(obj client.Object) (bool, string) {
	switch r := obj.(type) {
	case *keycloakv1beta1.KeycloakRealm:
		return r.Status.Ready, r.Status.RealmName
	case *keycloakv1beta1.ClusterKeycloakRealm:
		return r.Status.Ready, r.Status.RealmName
	}
	return false, ""
}
