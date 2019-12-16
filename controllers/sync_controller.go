package controllers

import (
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/ingress-nginx-next/pkg/ingress"
)

type Interface interface {
	EnsureIngress(ingress *networking.Ingress) ingress.StateHolder
	Services(ingress *networking.Ingress) []types.NamespacedName
	Configmaps(ingress *networking.Ingress) []types.NamespacedName
}

type SyncController struct {
}
