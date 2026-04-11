/*
Copyright 2026.

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

package v1

// Condition types for WorkloadProfile status.conditions.
const (
	// ConditionTypeTargetResolved is True when the referenced Deployment or StatefulSet exists and was read successfully.
	ConditionTypeTargetResolved = "TargetResolved"
	// ConditionTypeMetricsAvailable is True when the controller has processed kubelet stats for this reconcile.
	ConditionTypeMetricsAvailable = "MetricsAvailable"
	// ConditionTypeProfileReady is a composite condition: True when TargetResolved and MetricsAvailable are both True.
	ConditionTypeProfileReady = "ProfileReady"
)
