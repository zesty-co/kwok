/*
Copyright 2026 The Kubernetes Authors.

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

package cel

import (
	corev1 "k8s.io/api/core/v1"
	resourcehelper "k8s.io/component-helpers/resource"
)

func podRequests(pod *corev1.Pod) ResourceList {
	return NewResourceList(PodRequests(pod, false))
}

func podRequestsWithStatus(pod *corev1.Pod, useStatusResources bool) ResourceList {
	return NewResourceList(PodRequests(pod, useStatusResources))
}

// PodRequests returns the desired or current resource requests for a pod.
// Current requests use status resources when available and fall back to spec.
func PodRequests(pod *corev1.Pod, useStatusResources bool) corev1.ResourceList {
	if !useStatusResources {
		return resourcehelper.PodRequests(pod, resourcehelper.PodResourcesOptions{})
	}

	// Materialize current requests before aggregation: status values override
	// desired values, with spec as the fallback for missing resource names.
	// Only restartable init containers use status resources.
	pod = pod.DeepCopy()
	useCurrentContainerRequests(pod.Spec.Containers, pod.Status.ContainerStatuses, false)
	useCurrentContainerRequests(pod.Spec.InitContainers, pod.Status.InitContainerStatuses, true)
	if pod.Spec.Resources != nil {
		var actuated corev1.ResourceList
		if pod.Status.Resources != nil {
			actuated = pod.Status.Resources.Requests
		}
		pod.Spec.Resources.Requests = currentResourceRequests(pod.Spec.Resources.Requests, actuated, pod.Status.AllocatedResources)
	}

	return resourcehelper.PodRequests(pod, resourcehelper.PodResourcesOptions{})
}

func useCurrentContainerRequests(containers []corev1.Container, statuses []corev1.ContainerStatus, restartableOnly bool) {
	statusByName := make(map[string]corev1.ContainerStatus, len(statuses))
	for _, status := range statuses {
		statusByName[status.Name] = status
	}
	for i := range containers {
		if restartableOnly && (containers[i].RestartPolicy == nil || *containers[i].RestartPolicy != corev1.ContainerRestartPolicyAlways) {
			continue
		}
		status, ok := statusByName[containers[i].Name]
		if ok {
			var actuated corev1.ResourceList
			if status.Resources != nil {
				actuated = status.Resources.Requests
			}
			containers[i].Resources.Requests = currentResourceRequests(containers[i].Resources.Requests, actuated, status.AllocatedResources)
		}
	}
}

func currentResourceRequests(spec, actuated, allocated corev1.ResourceList) corev1.ResourceList {
	if spec == nil && (len(actuated) != 0 || len(allocated) != 0) {
		spec = corev1.ResourceList{}
	}
	for name, quantity := range actuated {
		spec[name] = quantity.DeepCopy()
	}
	for name, quantity := range allocated {
		if current, ok := actuated[name]; !ok || quantity.Cmp(current) > 0 {
			spec[name] = quantity.DeepCopy()
		}
	}
	return spec
}
