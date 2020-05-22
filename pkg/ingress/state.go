// Package ingress holds....
package ingress

import (
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

func NewDependenciesHolder() *Dependencies {
	return &Dependencies{make(map[types.NamespacedName]*Ingress)}
}

type Dependencies struct {
	data map[types.NamespacedName]*Ingress
}

func (s *Dependencies) Add(name types.NamespacedName, dependencies *Ingress) {
	s.data[name] = dependencies
}

func (s *Dependencies) Remove(name types.NamespacedName) {
	delete(s.data, name)
}

// Ingress contains information defined in an Ingress object
type Ingress struct {
	Services   []types.NamespacedName `json:"services"`
	Secrets    []types.NamespacedName `json:"secrets"`
	Configmaps []types.NamespacedName `json:"configmaps"`

	Annotations interface{} `json:"annotations"`
}

func Parse(ingress *networking.Ingress) *Ingress {
	return &Ingress{
		Services:   extractServices(ingress),
		Secrets:    extractSecrets(ingress),
		Configmaps: make([]types.NamespacedName, 0),
	}
}

func extractServices(ingress *networking.Ingress) []types.NamespacedName {
	services := []types.NamespacedName{}
	for _, rule := range ingress.Spec.Rules {
		for _, p := range rule.IngressRuleValue.HTTP.Paths {
			services = append(services, types.NamespacedName{
				Namespace: ingress.Namespace,
				Name:      p.Backend.ServiceName,
			})
		}
	}

	return services
}

func extractSecrets(ingress *networking.Ingress) []types.NamespacedName {
	if len(ingress.Spec.TLS) == 0 {
		return []types.NamespacedName{}
	}

	secrets := []types.NamespacedName{}
	for _, tls := range ingress.Spec.TLS {
		secrets = append(secrets, types.NamespacedName{
			Namespace: ingress.Namespace,
			Name:      tls.SecretName,
		})
	}

	return secrets
}
