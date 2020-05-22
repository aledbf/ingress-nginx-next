module k8s.io/ingress-nginx-next

go 1.14

require (
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.12.2
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v0.18.3
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.6.1-0.20200522180510-2f1457b4f505
)
