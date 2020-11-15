// Package ingress holds....
package ingress

import (
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Dependencies contains the information defined in an Ingress object
type Dependencies struct {
	Configmaps []types.NamespacedName `json:"configmaps"`
	Secrets    []types.NamespacedName `json:"secrets"`
	Services   []types.NamespacedName `json:"services"`
	Endpoints  []types.NamespacedName `json:"endpoints"`

	Annotations interface{} `json:"annotations"`
}

// Parse extracts information associated to the ingress definition:
// services, secrets and configmaps that should be watched because
// they are used in the TLS section or in an annotation
func Parse(ingress *networking.Ingress) *Dependencies {
	secrets := extractSecrets(ingress)
	secrets = append(secrets, secretsFromAnnotations(ingress)...)

	return &Dependencies{
		Services:    extractServices(ingress),
		Endpoints:   extractServices(ingress),
		Secrets:     secrets,
		Configmaps:  configmapsFromAnnotations(ingress),
		Annotations: extractAnnotations(ingress),
	}
}

func extractAnnotations(ingress *networking.Ingress) interface{} {
	return nil
}

// annotations that reference configmaps
var configmapAnnotations = sets.NewString(
	"auth-proxy-set-header",
	"fastcgi-params-configmap",
)

// configmapsFromAnnotations extracts a list of NamespacedName that reference Configmaps
func configmapsFromAnnotations(ingress *networking.Ingress) []types.NamespacedName {
	configmaps := make([]types.NamespacedName, 0)
	for name := range ingress.GetAnnotations() {
		if configmapAnnotations.Has(name) {
			configmaps = append(configmaps, types.NamespacedName{Namespace: ingress.Namespace, Name: name})
		}
	}

	return configmaps
}

// annotations that reference secrets
var secretsAnnotations = sets.NewString(
	"auth-secret",
	"auth-tls-secret",
	"proxy-ssl-secret",
	"secure-verify-ca-secret",
)

// secretsFromAnnotations extracts a list of NamespacedName that reference Secrets
func secretsFromAnnotations(ingress *networking.Ingress) []types.NamespacedName {
	secrets := make([]types.NamespacedName, 0)
	for name := range ingress.GetAnnotations() {
		if secretsAnnotations.Has(name) {
			secrets = append(secrets, types.NamespacedName{Namespace: ingress.Namespace, Name: name})
		}
	}

	return secrets
}

func extractServices(ingress *networking.Ingress) []types.NamespacedName {
	services := []types.NamespacedName{}
	for _, rule := range ingress.Spec.Rules {
		for _, p := range rule.IngressRuleValue.HTTP.Paths {
			services = append(services, types.NamespacedName{Namespace: ingress.Namespace, Name: p.Backend.ServiceName})
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
		secrets = append(secrets, types.NamespacedName{Namespace: ingress.Namespace, Name: tls.SecretName})
	}

	return secrets
}
