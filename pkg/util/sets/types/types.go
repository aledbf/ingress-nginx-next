// Package types just provides input types to the set generator. It also
// contains a "go generate" block.
// (You must first `go install k8s.io/code-generator/cmd/set-gen`)
package types

import "k8s.io/apimachinery/pkg/types"

// TODO: enable set-gen
// //go:generate set-gen -i k8s.io/ingress-nginx-next/pkg -p k8s.io/ingress-nginx-next/pkg/util/sets

type ReferenceSetTypes struct {
	// These types all cause files to be generated.
	// These types should be reflected in the output of
	// the "//pkg/util/sets:set-gen" genrule.
	a types.NamespacedName
}
