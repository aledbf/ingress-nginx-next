package watch

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Event holds information about an event that could change the state of the ingress controller
type Event struct {
	metav1.TypeMeta

	NamespacedName string
	Type           EventType
}

type EventType string

const (
	RemoveEvent    EventType = "Remove"
	AddUpdateEvent EventType = "AddUpdate"
)
