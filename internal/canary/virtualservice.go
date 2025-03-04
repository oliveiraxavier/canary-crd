package canary

import (
	"context"
	"fmt"

	"github.com/oliveiraxavier/canary-crd/api/v1alpha1"
	log "github.com/oliveiraxavier/canary-crd/internal/logs"

	istio "istio.io/client-go/pkg/apis/networking/v1alpha3"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetVirtualServiceForDeployment(clientSet *client.Client, appName string, namespace string) (*istio.VirtualService, error) {
	virtualService := &istio.VirtualService{}
	err := (*clientSet).Get(context.Background(), client.ObjectKey{Name: appName, Namespace: namespace}, virtualService)

	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Custom.Info("Virtual Service not found. The manifest possible deleted.")
			return nil, err
		}
		log.Custom.Error(err, "Err")
		log.Custom.Info("Error fetching Virtual Service" + appName)
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

			if subset == "stable" {
				stableWeight := route.Weight
				stableWeightStr := fmt.Sprintf("Virtual Service %s stable, has weight: %d", vs.Name, stableWeight)
				log.Custom.Info(stableWeightStr)
			}

			if subset == "canary" {
				canaryWeight := route.Weight
				canaryWeightStr := fmt.Sprintf("Virtual Service %s canary, has weight: %d", vs.Name, canaryWeight)
				log.Custom.Info(canaryWeightStr)
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

			if subset == "stable" {
				stableWeight := route.Weight
				stableNewWeight := 100 - weightCanary
				stableWeightStr := fmt.Sprintf("Virtual Service %s weight stable: %d", vs.Name, stableWeight)
				stableNewWeightStr := fmt.Sprintf("Virtual Service %s new weight stable after update: %d", vs.Name, stableNewWeight)
				log.Custom.Info(stableWeightStr)
				log.Custom.Info(stableNewWeightStr)
				vs.Spec.Http[i].Route[j].Weight = stableNewWeight
			}

			if subset == "canary" {
				canaryWeight := route.Weight
				canaryWeightStr := fmt.Sprintf("Virtual Service %s weight canary: %d", vs.Name, canaryWeight)
				canaryNewWeightStr := fmt.Sprintf("Virtual Service %s new weight canary after update: %d", vs.Name, weightCanary)
				log.Custom.Info(canaryWeightStr)
				log.Custom.Info(canaryNewWeightStr)
				vs.Spec.Http[i].Route[j].Weight = weightCanary
			}
		}
	}

	if originalVs.Spec.Http[0].Route[0].Weight == vs.Spec.Http[0].Route[0].Weight {
		log.Custom.Info("Skip Virtual service values why not changes")
		return vs, nil
	}

	err := (*clientSet).Patch(context.Background(), vs, client.MergeFrom(originalVs))

	if err != nil {
		return nil, err
	}

	return vs, nil
}

func UpdateVirtualServicePercentage(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment, namespace string) (*istio.VirtualService, error) {
	vs, err := GetVirtualServiceForDeployment(clientSet, canaryDeployment.Spec.AppName, namespace)

	if err == nil {
		ReconfigureWeightVirtualService(clientSet, vs, canaryDeployment.Spec.Steps[canaryDeployment.ActualStep-1].SetWeight)
		return vs, nil
	}

	return nil, err
}

func IsFullyPromoted(vs *istio.VirtualService) bool {
	fullyPromoted := false
	routesVs := vs.Spec.GetHttp()[0].Route
	for k, destination := range routesVs {

		if destination.Destination.Subset == "canary" {
			if routesVs[k].Weight == 100 {
				fullyPromoted = true
			}
		}
	}
	return fullyPromoted
}

func ResetFullPercentageToStable(clientSet *client.Client, canaryDeployment *v1alpha1.CanaryDeployment, namespace string) (*istio.VirtualService, error) {
	vs, err := GetVirtualServiceForDeployment(clientSet, canaryDeployment.Spec.AppName, namespace)

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
			log.Custom.Info("Skip Virtual service values why not changes")
			return vs, nil
		}

		err := (*clientSet).Patch(context.Background(), vs, client.MergeFrom(originalVs))

		if err != nil {
			return nil, err
		}
		log.Custom.Info("All traffic for virtual service " + canaryDeployment.Spec.AppName + " changed to 100% for stable deployment")
		return vs, nil
	}

	return nil, err
}
