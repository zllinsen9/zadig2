/*
Copyright 2021 The KodeRover Authors.

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

package updater

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DeleteConfigMaps(ns string, selector labels.Selector, cl client.Client) error {
	return deleteObjectsWithDefaultOptions(ns, selector, &corev1.ConfigMap{}, cl)
}

func UpdateConfigMap(cm *corev1.ConfigMap, cl client.Client) error {
	return updateObject(cm, cl)
}

func DeleteConfigMap(ns, name string, cl client.Client) error {
	return deleteObjectWithDefaultOptions(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}, cl)
}

func CreateConfigMap(cm *corev1.ConfigMap, cl client.Client) error {
	return createObject(cm, cl)
}

func DeleteConfigMapsAndWait(ns string, selector labels.Selector, cl client.Client) error {
	gvk := schema.GroupVersionKind{
		Group:   "",
		Kind:    "ConfigMap",
		Version: "v1",
	}
	return deleteObjectsAndWait(ns, selector, &corev1.ConfigMap{}, gvk, cl)
}
