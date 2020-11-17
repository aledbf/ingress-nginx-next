/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GetConfig returns a Kubernetes API client config
func GetConfig() (*rest.Config, error) {
	kubeCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	cfg, err := kubeCfg.ClientConfig()
	if err != nil {
		return nil, err
	}

	// TODO: remove after k8s v1.22
	cfg.WarningHandler = rest.NoWarnings{}

	return cfg, nil
}
