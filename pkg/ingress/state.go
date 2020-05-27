// Package ingress holds....
package ingress

import (
	"fmt"

	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Dependencies contains information defined in an Ingress object
type Dependencies struct {
	Services   []string `json:"services"`
	Secrets    []string `json:"secrets"`
	Configmaps []string `json:"configmaps"`

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
		Secrets:     secrets,
		Configmaps:  configmapsFromAnnotations(ingress),
		Annotations: extractAnnotations(ingress),
	}
}

func extractAnnotations(ingress *networking.Ingress) interface{} {
	return nil
}

var configmapAnnotations = sets.NewString(
	"auth-proxy-set-header",
	"fastcgi-params-configmap",
)

func configmapsFromAnnotations(ingress *networking.Ingress) []string {
	configmaps := make([]string, 0)
	for name := range ingress.GetAnnotations() {
		if configmapAnnotations.Has(name) {
			configmaps = append(configmaps, fmt.Sprintf("%v/%v", ingress.Namespace, name))
		}
	}

	return configmaps
}

var secretsAnnotations = sets.NewString(
	"auth-secret",
	"auth-tls-secret",
	"proxy-ssl-secret",
	"secure-verify-ca-secret",
)

func secretsFromAnnotations(ingress *networking.Ingress) []string {
	secrets := make([]string, 0)
	for name := range ingress.GetAnnotations() {
		if secretsAnnotations.Has(name) {
			secrets = append(secrets, fmt.Sprintf("%v/%v", ingress.Namespace, name))
		}
	}

	return secrets
}

func extractServices(ingress *networking.Ingress) []string {
	services := []string{}
	for _, rule := range ingress.Spec.Rules {
		for _, p := range rule.IngressRuleValue.HTTP.Paths {
			services = append(services, fmt.Sprintf("%v/%v", ingress.Namespace, p.Backend.ServiceName))
		}
	}

	return services
}

func extractSecrets(ingress *networking.Ingress) []string {
	if len(ingress.Spec.TLS) == 0 {
		return []string{}
	}

	secrets := []string{}
	for _, tls := range ingress.Spec.TLS {
		secrets = append(secrets, fmt.Sprintf("%v/%v", ingress.Namespace, tls.SecretName))
	}

	return secrets
}
