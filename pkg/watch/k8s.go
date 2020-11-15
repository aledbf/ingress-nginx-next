package watch

import (
	"context"
	"fmt"

	kcorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	scheme     = runtime.NewScheme()
	codecs     = serializer.NewCodecFactory(scheme)
	paramCodec = runtime.NewParameterCodec(scheme)
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

// newListWatch returns a new ListWatch object that can be used to create a SharedIndexInformer.
func createStructuredListWatch(key types.NamespacedName, gvk schema.GroupVersionKind, mapper meta.RESTMapper) (*cache.ListWatch, error) {
	restConfig, err := config.GetConfig()

	client, err := restClientForGVK(gvk, restConfig, codecs)
	if err != nil {
		return nil, err
	}

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	isNamespaceScoped := key.Namespace != kcorev1.NamespaceAll && mapping.Scope.Name() != meta.RESTScopeNameRoot

	ctx := context.TODO()
	return &cache.ListWatch{
		// setup the watch function
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.Watch = true
			// only watch one object
			opts.FieldSelector = fmt.Sprintf("metadata.name=%s", key.Name)
			// build watch
			return client.Get().NamespaceIfScoped(key.Namespace, isNamespaceScoped).Resource(mapping.Resource.Resource).VersionedParams(&opts, paramCodec).Watch(ctx)
		},
	}, nil
}

// restClientForGVK constructs a new rest.Interface capable of accessing the resource associated
// with the given GroupVersionKind. The REST client will be configured to use the negotiated serializer from
// baseConfig, if set, otherwise a default serializer will be set.
func restClientForGVK(gvk schema.GroupVersionKind, baseConfig *rest.Config, codecs serializer.CodecFactory) (rest.Interface, error) {
	cfg := createRestConfig(gvk, baseConfig)
	if cfg.NegotiatedSerializer == nil {
		cfg.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: codecs}
	}
	return rest.RESTClientFor(cfg)
}

//createRestConfig copies the base config and updates needed fields for a new rest config
func createRestConfig(gvk schema.GroupVersionKind, baseConfig *rest.Config) *rest.Config {
	gv := gvk.GroupVersion()

	cfg := rest.CopyConfig(baseConfig)
	cfg.GroupVersion = &gv

	if gvk.Group == "" {
		cfg.APIPath = "/api"
	} else {
		cfg.APIPath = "/apis"
	}

	if cfg.UserAgent == "" {
		cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return cfg
}
