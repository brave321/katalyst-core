/*
Copyright 2022 The Katalyst Authors.

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

package resource

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/metacache"
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/plugin/qosaware/resource/cpu"
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/plugin/qosaware/resource/memory"
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/types"
	"github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
)

// ResourceAdvisor is a wrapper of different sub resource advisors. It can be registered to
// headroom reporter to give designated resource headroom quantity based on provision result.
type ResourceAdvisor interface {
	// Update triggers update functions of all sub resource advisors
	Update()

	// GetSubAdvisor returns the corresponding sub advisor according to resource name
	GetSubAdvisor(resourceName types.QoSResourceName) (SubResourceAdvisor, error)

	// GetHeadroom returns the corresponding headroom quantity according to resource name
	GetHeadroom(resourceName v1.ResourceName) (resource.Quantity, error)
}

// SubResourceAdvisor updates resource provision of a certain dimension based on the latest
// system and workload snapshot(s), and returns provision advice or resource headroom quantity.
// It should push updated results to the corresponding qrm server.
type SubResourceAdvisor interface {
	// Name returns advisor name
	Name() string

	// Update updates resource provision based on the latest system and workload snapshot(s)
	Update()

	// GetChannel returns a channel to which the updated provision result will be sent
	GetChannel() interface{}

	// GetHeadroom returns the latest resource headroom quantity for resource reporter
	GetHeadroom() (resource.Quantity, error)
}

type resourceAdvisorWrapper struct {
	subAdvisorsToRun map[types.QoSResourceName]SubResourceAdvisor
}

// NewResourceAdvisor returns a resource advisor wrapper instance, initializing all required
// sub resource advisor according to config
func NewResourceAdvisor(conf *config.Configuration, metaCache *metacache.MetaCache,
	metaServer *metaserver.MetaServer, emitter metrics.MetricEmitter) (ResourceAdvisor, error) {
	resourceAdvisor := resourceAdvisorWrapper{
		subAdvisorsToRun: make(map[types.QoSResourceName]SubResourceAdvisor),
	}

	for _, resourceNameStr := range conf.ResourceAdvisors {
		resourceName := types.QoSResourceName(resourceNameStr)
		subAdvisor, err := NewSubResourceAdvisor(resourceName, conf, metaCache, metaServer, emitter)
		if err != nil {
			return nil, fmt.Errorf("new sub resource advisor for %v failed: %v", resourceName, err)
		}
		resourceAdvisor.subAdvisorsToRun[resourceName] = subAdvisor
	}

	return &resourceAdvisor, nil
}

// NewSubResourceAdvisor returns a corresponding advisor according to resource name
func NewSubResourceAdvisor(resourceName types.QoSResourceName, conf *config.Configuration,
	metaCache *metacache.MetaCache, metaServer *metaserver.MetaServer, emitter metrics.MetricEmitter) (SubResourceAdvisor, error) {
	switch resourceName {
	case types.QoSResourceCPU:
		return cpu.NewCPUResourceAdvisor(conf, metaCache, metaServer, emitter)
	case types.QoSResourceMemory:
		return memory.NewMemoryResourceAdvisor(conf, metaCache, metaServer, emitter)
	default:
		return nil, fmt.Errorf("try to new unknown resource advisor: %v", resourceName)
	}
}

func (ra *resourceAdvisorWrapper) Update() {
	for _, subAdvisor := range ra.subAdvisorsToRun {
		subAdvisor.Update()
	}
}

func (ra *resourceAdvisorWrapper) GetSubAdvisor(resourceName types.QoSResourceName) (SubResourceAdvisor, error) {
	if subAdvisor, ok := ra.subAdvisorsToRun[resourceName]; ok {
		return subAdvisor, nil
	}
	return nil, fmt.Errorf("no sub resource advisor for %v", resourceName)
}

func (ra *resourceAdvisorWrapper) GetHeadroom(resourceName v1.ResourceName) (resource.Quantity, error) {
	switch resourceName {
	case v1.ResourceCPU:
		return ra.getSubAdvisorHeadroom(types.QoSResourceCPU)
	case v1.ResourceMemory:
		return ra.getSubAdvisorHeadroom(types.QoSResourceMemory)
	default:
		return resource.Quantity{}, fmt.Errorf("illegal resource %v", resourceName)
	}
}

func (ra *resourceAdvisorWrapper) getSubAdvisorHeadroom(resourceName types.QoSResourceName) (resource.Quantity, error) {
	subAdvisor, ok := ra.subAdvisorsToRun[resourceName]
	if !ok {
		return resource.Quantity{}, fmt.Errorf("no sub resource advisor for %v", resourceName)
	}
	return subAdvisor.GetHeadroom()
}
