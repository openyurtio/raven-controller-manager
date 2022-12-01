/*
Copyright 2022 The OpenYurt Authors.

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

package dnscontroller

type EventType string

const (
	NodeAdd       EventType = "NODE_ADD"
	NodeUpdate    EventType = "NODE_UPDATE"
	NodeDelete    EventType = "NODE_DELETE"
	ServiceAdd    EventType = "SERVICE_ADD"
	ServiceUpdate EventType = "SERVICE_UPDATE"
	PodAdd        EventType = "Pod_ADD"
	PodUpdate     EventType = "Pod_UPDATE"
	PodDelete     EventType = "Pod_DELETE"
)

type Event struct {
	Type EventType
	Obj  interface{}
}
