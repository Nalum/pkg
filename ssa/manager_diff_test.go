/*
Copyright 2021 Stefan Prodan
Copyright 2021 The Flux authors

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

package ssa

import (
	"context"
	"sigs.k8s.io/yaml"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDiff(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("diff")
	objects, err := readManifest("testdata/test1.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	configMapName, configMap := getFirstObject(objects, "ConfigMap", id)
	secretName, secret := getFirstObject(objects, "Secret", id)

	if err := unstructured.SetNestedField(secret.Object, false, "immutable"); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions()); err != nil {
		t.Fatal(err)
	}

	t.Run("generates empty diff for unchanged object", func(t *testing.T) {
		changeSetEntry, _, _, err := manager.Diff(ctx, configMap)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(configMapName, changeSetEntry.Subject); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if diff := cmp.Diff(string(UnchangedAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})

	t.Run("generates diff for changed object", func(t *testing.T) {
		newVal := "diff-test"
		err = unstructured.SetNestedField(configMap.Object, newVal, "data", "key")
		if err != nil {
			t.Fatal(err)
		}

		changeSetEntry, _, mergedObj, err := manager.Diff(ctx, configMap)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(string(ConfiguredAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		mergedObjYaml, _ := yaml.Marshal(mergedObj)
		if !strings.Contains(string(mergedObjYaml), newVal) {
			t.Errorf("Mismatch from expected value, want %s", newVal)
		}
	})

	t.Run("masks secret values", func(t *testing.T) {
		newVal := "diff-test"
		err = unstructured.SetNestedField(secret.Object, newVal, "stringData", "key")
		if err != nil {
			t.Fatal(err)
		}

		newKey := "key.new"
		err = unstructured.SetNestedField(secret.Object, newVal, "stringData", newKey)
		if err != nil {
			t.Fatal(err)
		}

		changeSetEntry, _, mergedObj, err := manager.Diff(ctx, secret)
		if err != nil {
			t.Fatal(err)
		}

		mergedObjYaml, _ := yaml.Marshal(mergedObj)

		if diff := cmp.Diff(secretName, changeSetEntry.Subject); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if !strings.Contains(string(mergedObjYaml), newKey) {
			t.Errorf("Mismatch from expected value, got %s", string(mergedObjYaml))
		}

		if strings.Contains(string(mergedObjYaml), newVal) {
			t.Errorf("Mismatch from expected value, got %s", string(mergedObjYaml))
		}
	})
}

func TestDiff_Removals(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("diff")
	objects, err := readManifest("testdata/test4.yaml", id)
	if err != nil {
		t.Fatal(err)
	}
	SetNativeKindsDefaults(objects)

	configMapName, configMap := getFirstObject(objects, "ConfigMap", id)

	if _, err = manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions()); err != nil {
		t.Fatal(err)
	}

	t.Run("generates empty diff for unchanged object", func(t *testing.T) {
		changeSetEntry, _, _, err := manager.Diff(ctx, configMap)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(configMapName, changeSetEntry.Subject); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if diff := cmp.Diff(string(UnchangedAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if _, err = manager.ApplyAll(ctx, []*unstructured.Unstructured{configMap}, DefaultApplyOptions()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("generates diff for added map entry", func(t *testing.T) {
		newVal := "diff-test"
		err = unstructured.SetNestedField(configMap.Object, newVal, "data", "token")
		if err != nil {
			t.Fatal(err)
		}

		changeSetEntry, _, mergedObj, err := manager.Diff(ctx, configMap)
		if err != nil {
			t.Fatal(err)
		}

		mergedObjYaml, _ := yaml.Marshal(mergedObj)

		if diff := cmp.Diff(string(ConfiguredAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if !strings.Contains(string(mergedObjYaml), newVal) {
			t.Errorf("Mismatch from expected value, want %s", newVal)
		}

		if _, err = manager.ApplyAll(ctx, []*unstructured.Unstructured{configMap}, DefaultApplyOptions()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("generates diff for removed map entry", func(t *testing.T) {
		unstructured.RemoveNestedField(configMap.Object, "data", "token")

		changeSetEntry, _, _, err := manager.Diff(ctx, configMap)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(string(ConfiguredAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if _, err = manager.ApplyAll(ctx, []*unstructured.Unstructured{configMap}, DefaultApplyOptions()); err != nil {
			t.Fatal(err)
		}
	})

}

func TestDiffHPA(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("diff")
	objects, err := readManifest("testdata/test6.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	hpaName, hpa := getFirstObject(objects, "HorizontalPodAutoscaler", id)
	var metrics []interface{}

	if _, err = manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions()); err != nil {
		t.Fatal(err)
	}

	t.Run("generates empty diff for unchanged object", func(t *testing.T) {
		changeSetEntry, _, _, err := manager.Diff(ctx, hpa)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(hpaName, changeSetEntry.Subject); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if diff := cmp.Diff(string(UnchangedAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})

	t.Run("generates diff for removed metric", func(t *testing.T) {
		metrics, _, err = unstructured.NestedSlice(hpa.Object, "spec", "metrics")
		if err != nil {
			t.Fatal(err)
		}

		err = unstructured.SetNestedSlice(hpa.Object, metrics[:1], "spec", "metrics")
		if err != nil {
			t.Fatal(err)
		}

		changeSetEntry, _, _, err := manager.Diff(ctx, hpa)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(string(ConfiguredAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if _, err = manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("generates empty diff for unchanged metric", func(t *testing.T) {
		changeSetEntry, _, _, err := manager.Diff(ctx, hpa)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(string(UnchangedAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})

	t.Run("generates diff for added metric", func(t *testing.T) {
		err = unstructured.SetNestedSlice(hpa.Object, metrics, "spec", "metrics")
		if err != nil {
			t.Fatal(err)
		}

		changeSetEntry, _, _, err := manager.Diff(ctx, hpa)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(string(ConfiguredAction), changeSetEntry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}

		if _, err = manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions()); err != nil {
			t.Fatal(err)
		}
	})
}