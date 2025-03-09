package canary

import (
	"context"
	"fmt"

	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	log "github.com/oliveiraxavier/canary-crd/internal/logs"
	istio "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stableText = "stable"
	canaryText = "canary"
)

func GetVirtualServiceForDeployment(clientSet *client.Client, istioVirtualServiceName string, namespace string) (*istio.VirtualService, error) {
	// TODO add app name based in spec
	virtualService := &istio.VirtualService{}
	err := (*clientSet).Get(context.Background(), client.ObjectKey{Name: istioVirtualServiceName, Namespace: namespace}, virtualService)

	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Custom.Info("Virtual Service not found. The manifest possible deleted.", "virtual service", istioVirtualServiceName)
			return nil, err
		}
		log.Custom.Error(err, "Err")
		log.Custom.Info("Error fetching Virtual Service", "virtual service", istioVirtualServiceName)
		return nil, err
	}
	return virtualService, nil
}

func GetWeightForVirtualService(clientSet *client.Client, vs *istio.VirtualService) (*istio.VirtualService, int, int, error) {
	if vs.Spec.Http == nil {
		return nil, 0, 0, fmt.Errorf("http routes are empty. Please check the Virtual Service")
	}
	var stableWeight int
	var canaryWeight int
	for _, httpRoute := range vs.Spec.Http {
		for _, route := range httpRoute.Route {
			host := route.Destination.Host
			subset := route.Destination.Subset
			log.Custom.Info("Host: " + host)
			log.Custom.Info("Subset: " + subset)

			if subset == stableText {
				stableWeight := route.Weight
				log.Custom.Info("Virtual Service stable, has weight:", vs.Name, stableWeight)
			}

			if subset == canaryText {
				canaryWeight := route.Weight
				log.Custom.Info("Virtual Service canary, has weight:", vs.Name, canaryWeight)
			}
		}
	}

	return vs, stableWeight, canaryWeight, nil
}

func ReconfigureWeightVirtualService(clientSet *client.Client, vs *istio.VirtualService, weightCanary int32) (*istio.VirtualService, error) {
	originalVs := vs.DeepCopy()
	if vs.Spec.Http == nil {
		return nil, fmt.Errorf("http routes are empty. Please check the Virtual Service")
	}

	for i, httpRoute := range vs.Spec.Http {
		for j, route := range httpRoute.Route {
			subset := route.Destination.Subset

			if subset == stableText {
				stableWeight := route.Weight
				stableNewWeight := 100 - weightCanary
				log.Custom.Info("############## Stable " + vs.Name + " ##############")
				log.Custom.Info("Virtual Service current weight stable", "name", vs.Name, "weight", stableWeight)
				log.Custom.Info("Virtual Service weight stable after updated", "name", vs.Name, "new weight", stableNewWeight)
				vs.Spec.Http[i].Route[j].Weight = stableNewWeight
			}

			if subset == canaryText {
				canaryWeight := route.Weight
				log.Custom.Info("############## Canary " + vs.Name + " ##############")
				log.Custom.Info("Virtual Service current weight canary", "name", vs.Name, "weight", canaryWeight)
				log.Custom.Info("Virtual Service weight canary after update", "name", vs.Name, "new weight", weightCanary)
				vs.Spec.Http[i].Route[j].Weight = weightCanary
			}
		}
	}

	if originalVs.Spec.Http[0].Route[0].Weight == vs.Spec.Http[0].Route[0].Weight {
		log.Custom.Info("Skip Virtual service values why not changes", "virtual service", vs.Name)
		return vs, nil
	}

	err := (*clientSet).Patch(context.Background(), vs, client.MergeFrom(originalVs))

	if err != nil {
		return nil, err
	}

	return vs, nil
}

func UpdateVirtualServicePercentage(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment, namespace string) (*istio.VirtualService, error) {
	vs, err := GetVirtualServiceForDeployment(clientSet, canaryDeployment.Spec.IstioVirtualServiceName, namespace)

	if err == nil {
		_, err = ReconfigureWeightVirtualService(clientSet, vs, canaryDeployment.Spec.Steps[canaryDeployment.CurrentStep-1].SetWeight)
		return vs, err
	}

	return nil, err
}

func IsFullyPromoted(vs *istio.VirtualService) bool {
	fullyPromoted := false
	routesVs := vs.Spec.GetHttp()[0].Route
	for k, destination := range routesVs {

		if destination.Destination.Subset == canaryText {
			if routesVs[k].Weight == 100 {
				fullyPromoted = true
			}
		}
	}
	return fullyPromoted
}

func ResetFullPercentageToStable(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment, namespace string) (*istio.VirtualService, error) {
	vs, err := GetVirtualServiceForDeployment(clientSet, canaryDeployment.Spec.IstioVirtualServiceName, namespace)

	if err == nil {
		originalVs := vs.DeepCopy()

		if vs.Spec.Http == nil {
			return nil, fmt.Errorf("http routes are empty. Please check the Virtual Service")
		}

		for i, httpRoute := range vs.Spec.Http {
			for j, route := range httpRoute.Route {
				subset := route.Destination.Subset

				if subset == "stable" {
					vs.Spec.Http[i].Route[j].Weight = 100
				}

				if subset == "canary" {
					vs.Spec.Http[i].Route[j].Weight = 0
				}
			}
		}

		if originalVs.Spec.Http[0].Route[0].Weight == vs.Spec.Http[0].Route[0].Weight {
			log.Custom.Info("Skip Virtual service values why not changes in reset all for stable version", "virtual service", vs.Name, "app", canaryDeployment.Spec.AppName)
			return vs, nil
		}

		err := (*clientSet).Patch(context.Background(), vs, client.MergeFrom(originalVs))

		if err != nil {
			return nil, err
		}
		log.Custom.Info("All traffic for virtual service changed to 100% for stable deployment", "virtual service", vs.Name, "app", canaryDeployment.Spec.AppName)
		return vs, nil
	}

	return nil, err
}
