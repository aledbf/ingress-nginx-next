package annotations

import (
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/ingress-nginx-next/pkg/k8s/ingress/annotations/parser"
)

type Ingress struct {
	types.NamespacedName
}

// Extractor defines the annotation parsers to be used in the extraction of annotations
type Extractor struct {
	annotations map[string]parser.IngressAnnotation
}

// NewAnnotationExtractor creates a new annotations extractor
func NewAnnotationExtractor() Extractor {
	return Extractor{
		map[string]parser.IngressAnnotation{},
	}
}

// Extract extracts the annotations from an Ingress
func (e Extractor) Extract(ingress *networking.Ingress) *Ingress {
	pia := &Ingress{
		NamespacedName: types.NamespacedName{
			Name:      ingress.Name,
			Namespace: ingress.Namespace,
		},
	}

	data := make(map[string]interface{})
	for name, annotationParser := range e.annotations {
		val, err := annotationParser.Parse(ingress)
		if err != nil {
			/*
				if errors.IsMissingAnnotations(err) {
					continue
				}

				if !errors.IsLocationDenied(err) {
					continue
				}

				_, alreadyDenied := data[DeniedKeyName]
				if !alreadyDenied {
					errString := err.Error()
					data[DeniedKeyName] = &errString
					klog.Errorf("error reading %v annotation in Ingress %v/%v: %v", name, ing.GetNamespace(), ing.GetName(), err)
					continue
				}

				klog.V(5).Infof("error reading %v annotation in Ingress %v/%v: %v", name, ing.GetNamespace(), ing.GetName(), err)
			*/

		}

		if val != nil {
			data[name] = val
		}
	}

	return pia
}
