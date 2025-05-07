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

package provider

import (
	"sigs.k8s.io/external-dns/endpoint"
)

// BaseProvider implements methods of provider interface that are commonly "ignored" by dns providers
// Basic implementation of the methods is done to avoid code repetition
type BaseProvider struct {
	domainFilter endpoint.DomainFilter
}

// NewBaseProvider returns an instance of new BaseProvider
func NewBaseProvider(domainFilter endpoint.DomainFilter) *BaseProvider {
	return &BaseProvider{domainFilter}
}

// GetDomainFilter basic implementation using the common domainFilter attribute
func (b BaseProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	return b.domainFilter
}

// AdjustEndpoints basic implementation of provider interface method
func (b BaseProvider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	return endpoints, nil
}
