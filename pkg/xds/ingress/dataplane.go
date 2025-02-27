package ingress

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	mesh_proto "github.com/Kong/kuma/api/mesh/v1alpha1"
	core_mesh "github.com/Kong/kuma/pkg/core/resources/apis/mesh"
	"github.com/Kong/kuma/pkg/core/resources/manager"
	"github.com/Kong/kuma/pkg/xds/envoy"
)

// tagSets represent map from tags (encoded as string) to number of instances
type tagSets map[serviceKey]uint32

type serviceKey struct {
	mesh string
	tags string
}

type serviceKeySlice []serviceKey

func (s serviceKeySlice) Len() int           { return len(s) }
func (s serviceKeySlice) Less(i, j int) bool { return s[i].mesh < s[j].mesh || s[i].tags < s[j].tags }
func (s serviceKeySlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (sk *serviceKey) String() string {
	return fmt.Sprintf("%s.%s", sk.tags, sk.mesh)
}

func (s tagSets) addInstanceOfTags(mesh string, tags envoy.Tags) {
	strTags := tags.String()
	s[serviceKey{tags: strTags, mesh: mesh}]++
}

func (s tagSets) toAvailableServices() []*mesh_proto.Dataplane_Networking_Ingress_AvailableService {
	var result []*mesh_proto.Dataplane_Networking_Ingress_AvailableService

	var keys []serviceKey
	for key := range s {
		keys = append(keys, key)
	}
	sort.Sort(serviceKeySlice(keys))

	for _, key := range keys {
		tags, _ := envoy.TagsFromString(key.tags) // ignore error since we control how string looks like
		result = append(result, &mesh_proto.Dataplane_Networking_Ingress_AvailableService{
			Tags:      tags,
			Instances: s[key],
			Mesh:      key.mesh,
		})
	}
	return result
}

func UpdateAvailableServices(ctx context.Context, rm manager.ResourceManager, ingress *core_mesh.DataplaneResource, others []*core_mesh.DataplaneResource) error {
	availableServices := GetIngressAvailableServices(others)
	if reflect.DeepEqual(availableServices, ingress.Spec.GetNetworking().GetIngress().GetAvailableServices()) {
		return nil
	}
	ingress.Spec.Networking.Ingress.AvailableServices = availableServices
	if err := rm.Update(ctx, ingress); err != nil {
		return err
	}
	return nil
}

func GetIngressAvailableServices(others []*core_mesh.DataplaneResource) []*mesh_proto.Dataplane_Networking_Ingress_AvailableService {
	tagSets := tagSets{}
	for _, dp := range others {
		if dp.Spec.IsIngress() {
			continue
		}
		for _, dpInbound := range dp.Spec.GetNetworking().GetInbound() {
			tagSets.addInstanceOfTags(dp.GetMeta().GetMesh(), dpInbound.Tags)
		}
	}
	return tagSets.toAvailableServices()
}
