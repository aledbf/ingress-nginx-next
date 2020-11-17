package cache

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Event holds information about an event that could change the state of the ingress controller
type Event struct {
	metav1.TypeMeta

	// NamespacedName namespace and name of the source of the event
	NamespacedName types.NamespacedName `json:"namespacedName"`

	Type EventType `json:"type"`

	// Object Kubernetes Object source of the event
	// +optional
	Object runtime.Object `json:"object,omitempty"`
}

// EventType defines the possible types of events.
type EventType string

const (
	// RemoveEvent object removal
	RemoveEvent EventType = "Remove"
	// AddOrUpdateEvent new object or update
	AddOrUpdateEvent EventType = "AddOrUpdate"
)
