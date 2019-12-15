// Package ingress holds....
package ingress

import (
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

type StateHolder struct {
	Services    []types.NamespacedName
	Secrets     []networking.IngressTLS
	Configmaps  []types.NamespacedName
	Annotations interface{}
}

func New() *State {
	return &State{
		state: make(map[types.NamespacedName]StateHolder),
	}
}

type State struct {
	state map[types.NamespacedName]StateHolder
}

func (i State) Ensure(name types.NamespacedName, ingress *networking.Ingress) StateHolder {
	state, ok := i.state[name]
	if !ok {
		i.state[name] = StateHolder{}
		state = i.state[name]
	}

	state.Services = extractServices(ingress)
	state.Secrets = extractSecrets(ingress)
	state.Configmaps = make([]types.NamespacedName, 0)

	return state
}

func (i State) Remove(names ...types.NamespacedName) {
	for _, item := range names {
		delete(i.state, item)
	}
}

func extractServices(ingress *networking.Ingress) []types.NamespacedName {
	res := []types.NamespacedName{}
	for _, rule := range ingress.Spec.Rules {
		for _, p := range rule.IngressRuleValue.HTTP.Paths {
			res = append(res, types.NamespacedName{
				Namespace: ingress.Namespace,
				Name:      p.Backend.ServiceName,
			})
		}
	}

	return res
}

func extractSecrets(ingress *networking.Ingress) []networking.IngressTLS {
	if len(ingress.Spec.TLS) == 0 {
		return make([]networking.IngressTLS, 0)
	}

	return ingress.Spec.TLS
}
