// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"strings"

	"github.com/golang/glog"
	swfregister "github.com/kubeflow/pipelines/backend/src/crd/pkg/apis/scheduledworkflow"
	swfapi "github.com/kubeflow/pipelines/backend/src/crd/pkg/apis/scheduledworkflow/v1beta1"
	workflowapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
)

// Workflow is a type to help manipulate Workflow objects.
type Workflow struct {
	*workflowapi.PipelineRun
}

// NewWorkflow creates a Workflow.
func NewWorkflow(workflow *workflowapi.PipelineRun) *Workflow {
	return &Workflow{
		workflow,
	}
}

// SetServiceAccount Set the service account to run the workflow.
func (w *Workflow) SetServiceAccount(serviceAccount string) {
	w.Spec.ServiceAccountName = serviceAccount
}

// OverrideParameters overrides some of the parameters of a Workflow.
func (w *Workflow) OverrideParameters(desiredParams map[string]string) {
	desiredSlice := make([]workflowapi.Param, 0)
	for _, currentParam := range w.Spec.Params {
		var desiredValue workflowapi.ArrayOrString = workflowapi.ArrayOrString{
			Type:      "string",
			StringVal: "",
		}
		if param, ok := desiredParams[currentParam.Name]; ok {
			desiredValue.StringVal = param
		} else {
			desiredValue.StringVal = currentParam.Value.StringVal
		}
		desiredSlice = append(desiredSlice, workflowapi.Param{
			Name:  currentParam.Name,
			Value: desiredValue,
		})
	}
	w.Spec.Params = desiredSlice
}

func (w *Workflow) VerifyParameters(desiredParams map[string]string) error {
	templateParamsMap := make(map[string]*string)
	for _, param := range w.Spec.Params {
		templateParamsMap[param.Name] = &param.Value.StringVal
	}
	for k := range desiredParams {
		_, ok := templateParamsMap[k]
		if !ok {
			return NewInvalidInputError("Unrecognized input parameter: %v", k)
		}
	}
	return nil
}

// Get converts this object to a workflowapi.Workflow.
func (w *Workflow) Get() *workflowapi.PipelineRun {
	return w.PipelineRun
}

func (w *Workflow) ScheduledWorkflowUUIDAsStringOrEmpty() string {
	if w.OwnerReferences == nil {
		return ""
	}

	for _, reference := range w.OwnerReferences {
		if isScheduledWorkflow(reference) {
			return string(reference.UID)
		}
	}

	return ""
}

func containsScheduledWorkflow(references []metav1.OwnerReference) bool {
	if references == nil {
		return false
	}

	for _, reference := range references {
		if isScheduledWorkflow(reference) {
			return true
		}
	}

	return false
}

func isScheduledWorkflow(reference metav1.OwnerReference) bool {
	gvk := schema.GroupVersionKind{
		Group:   swfapi.SchemeGroupVersion.Group,
		Version: swfapi.SchemeGroupVersion.Version,
		Kind:    swfregister.Kind,
	}

	if reference.APIVersion == gvk.GroupVersion().String() &&
		reference.Kind == gvk.Kind &&
		reference.UID != "" {
		return true
	}
	return false
}

func (w *Workflow) ScheduledAtInSecOr0() int64 {
	if w.Labels == nil {
		return 0
	}

	for key, value := range w.Labels {
		if key == LabelKeyWorkflowEpoch {
			result, err := RetrieveInt64FromLabel(value)
			if err != nil {
				glog.Errorf("Could not retrieve scheduled epoch from label key (%v) and label value (%v).", key, value)
				return 0
			}
			return result
		}
	}

	return 0
}

func (w *Workflow) FinishedAt() int64 {
	if w.Status.PipelineRunStatusFields.CompletionTime.IsZero() {
		// If workflow is not finished
		return 0
	}
	return w.Status.PipelineRunStatusFields.CompletionTime.Unix()
}

func (w *Workflow) Condition() string {
	if len(w.Status.Status.Conditions) > 0 {
		return string(w.Status.Status.Conditions[0].Reason)
	} else {
		return ""
	}
}

func (w *Workflow) ToStringForStore() string {
	workflow, err := json.Marshal(w.PipelineRun)
	if err != nil {
		glog.Errorf("Could not marshal the workflow: %v", w.PipelineRun)
		return ""
	}
	return string(workflow)
}

func (w *Workflow) HasScheduledWorkflowAsParent() bool {
	return containsScheduledWorkflow(w.PipelineRun.OwnerReferences)
}

func (w *Workflow) GetWorkflowSpec() *Workflow {
	workflow := w.DeepCopy()
	workflow.Status = workflowapi.PipelineRunStatus{}
	workflow.TypeMeta = metav1.TypeMeta{Kind: w.Kind, APIVersion: w.APIVersion}
	// To prevent collisions, clear name, set GenerateName to first 200 runes of previous name.
	nameRunes := []rune(w.Name)
	length := len(nameRunes)
	if length > 200 {
		length = 200
	}
	workflow.ObjectMeta = metav1.ObjectMeta{GenerateName: string(nameRunes[:length])}
	return NewWorkflow(workflow)
}

// OverrideName sets the name of a Workflow.
func (w *Workflow) OverrideName(name string) {
	w.GenerateName = ""
	w.Name = name
}

// SetAnnotations sets annotations on all templates in a Workflow
func (w *Workflow) SetAnnotationsToAllTemplates(key string, value string) {
	// No metadata object within pipelineRun task
	return
}

// SetLabels sets labels on all templates in a Workflow
func (w *Workflow) SetLabelsToAllTemplates(key string, value string) {
	// No metadata object within pipelineRun task
	return
}

// SetOwnerReferences sets owner references on a Workflow.
func (w *Workflow) SetOwnerReferences(schedule *swfapi.ScheduledWorkflow) {
	w.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(schedule, schema.GroupVersionKind{
			Group:   swfapi.SchemeGroupVersion.Group,
			Version: swfapi.SchemeGroupVersion.Version,
			Kind:    swfregister.Kind,
		}),
	}
}

func (w *Workflow) SetLabels(key string, value string) {
	if w.Labels == nil {
		w.Labels = make(map[string]string)
	}
	w.Labels[key] = value
}

func (w *Workflow) SetAnnotations(key string, value string) {
	if w.Annotations == nil {
		w.Annotations = make(map[string]string)
	}
	w.Annotations[key] = value
}

func (w *Workflow) ReplaceUID(id string) error {
	newWorkflowString := strings.Replace(w.ToStringForStore(), "{{workflow.uid}}", id, -1)
	var workflow *workflowapi.PipelineRun
	if err := json.Unmarshal([]byte(newWorkflowString), &workflow); err != nil {
		return NewInternalServerError(err,
			"Failed to unmarshal workflow spec manifest. Workflow: %s", w.ToStringForStore())
	}
	w.PipelineRun = workflow
	return nil
}

func (w *Workflow) SetCannonicalLabels(name string, nextScheduledEpoch int64, index int64) {
	w.SetLabels(LabelKeyWorkflowScheduledWorkflowName, name)
	w.SetLabels(LabelKeyWorkflowEpoch, FormatInt64ForLabel(nextScheduledEpoch))
	w.SetLabels(LabelKeyWorkflowIndex, FormatInt64ForLabel(index))
	w.SetLabels(LabelKeyWorkflowIsOwnedByScheduledWorkflow, "true")
}

// FindObjectStoreArtifactKeyOrEmpty loops through all node running statuses and look up the first
// S3 artifact with the specified nodeID and artifactName. Returns empty if nothing is found.
func (w *Workflow) FindObjectStoreArtifactKeyOrEmpty(nodeID string, artifactName string) string {
	// TODO: Fix the below code for Tekton to enable artifact support.

	// if w.Status.Nodes == nil {
	// 	return ""
	// }
	// node, found := w.Status.Nodes[nodeID]
	// if !found {
	// 	return ""
	// }
	// if node.Outputs == nil || node.Outputs.Artifacts == nil {
	// 	return ""
	// }
	// var s3Key string
	// for _, artifact := range node.Outputs.Artifacts {
	// 	if artifact.Name != artifactName || artifact.S3 == nil || artifact.S3.Key == "" {
	// 		continue
	// 	}
	// 	s3Key = artifact.S3.Key
	// }
	// return s3Key
	return ""
}

// IsInFinalState whether the workflow is in a final state.
func (w *Workflow) IsInFinalState() bool {
	if len(w.Status.Status.Conditions) > 0 {
		if w.Status.Status.Conditions[0].Reason == "Succeeded" || w.Status.Status.Conditions[0].Reason == "Failed" {
			return true
		}
	}
	return false
}

// PersistedFinalState whether the workflow final state has being persisted.
func (w *Workflow) PersistedFinalState() bool {
	if _, ok := w.GetLabels()[LabelKeyWorkflowPersistedFinalState]; ok {
		// If the label exist, workflow final state has being persisted.
		return true
	}
	return false
}
