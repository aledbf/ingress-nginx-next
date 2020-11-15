package watch

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Event holds information about an event that could change the state of the ingress controller
type Event struct {
	metav1.TypeMeta

	NamespacedName types.NamespacedName
	Type           EventType
}

type EventType string

const (
	RemoveEvent      EventType = "Remove"
	AddOrUpdateEvent EventType = "AddOrUpdate"
)
