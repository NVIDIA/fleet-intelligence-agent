// Package faultinjector provides a way to inject failures into the system.
package faultinjector

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	componentsnvidiaxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
)

// Injector defines the interface for injecting failures into the system.
type Injector interface {
	KmsgWriter() pkgkmsgwriter.KmsgWriter
}

// ComponentErrorInjector defines the interface for injecting component errors.
type ComponentErrorInjector interface {
	InjectComponentError(ctx context.Context, registry interface{}, componentError *ComponentError) error
}

// EventInjector defines the interface for injecting events.
type EventInjector interface {
	InjectEvent(ctx context.Context, registry interface{}, eventToInject *EventToInject) error
}

// ComponentClearInjector defines the interface for clearing component faults.
type ComponentClearInjector interface {
	ClearComponentFault(ctx context.Context, registry interface{}, componentClear *ComponentClear) error
}

func NewInjector(kmsgWriter pkgkmsgwriter.KmsgWriter) Injector {
	return &injector{
		kmsgWriter: kmsgWriter,
	}
}

type injector struct {
	kmsgWriter pkgkmsgwriter.KmsgWriter
}

func (i *injector) KmsgWriter() pkgkmsgwriter.KmsgWriter {
	return i.kmsgWriter
}

func (i *injector) InjectComponentError(ctx context.Context, registry interface{}, componentError *ComponentError) error {
	// Use reflection to get the component from the registry
	registryValue := reflect.ValueOf(registry)
	getMethod := registryValue.MethodByName("Get")
	if !getMethod.IsValid() {
		return fmt.Errorf("registry does not have Get method")
	}

	// Call registry.Get(componentName)
	results := getMethod.Call([]reflect.Value{reflect.ValueOf(componentError.Component)})
	if len(results) == 0 {
		return fmt.Errorf("registry Get method returned no results")
	}

	component := results[0]
	if component.IsNil() {
		return fmt.Errorf("component '%s' not found in registry", componentError.Component)
	}

	// Try different injection methods based on component type
	componentValue := component.Elem() // Get the underlying component

	// Call the standardized InjectFault method on any component
	method := componentValue.MethodByName("InjectFault")
	if !method.IsValid() {
		return fmt.Errorf("component %s does not have InjectFault method", componentError.Component)
	}

	method.Call([]reflect.Value{reflect.ValueOf(componentError.Message)})
	return nil
}

func (i *injector) InjectEvent(ctx context.Context, registry interface{}, eventToInject *EventToInject) error {
	// Use reflection to get the component from the registry (same pattern as component error injection)
	registryValue := reflect.ValueOf(registry)
	getMethod := registryValue.MethodByName("Get")
	if !getMethod.IsValid() {
		return fmt.Errorf("registry does not have Get method")
	}

	// Call registry.Get(componentName)
	results := getMethod.Call([]reflect.Value{reflect.ValueOf(eventToInject.Component)})
	if len(results) == 0 {
		return fmt.Errorf("registry Get method returned no results")
	}

	component := results[0]
	if component.IsNil() {
		return fmt.Errorf("component '%s' not found in registry", eventToInject.Component)
	}

	// Get the underlying component
	componentValue := component.Elem()

	// Call the standardized InjectEvent method on any component
	method := componentValue.MethodByName("InjectEvent")
	if !method.IsValid() {
		return fmt.Errorf("component %s does not have InjectEvent method", eventToInject.Component)
	}

	// Call the injection method with the event parameters
	callResults := method.Call([]reflect.Value{
		reflect.ValueOf(eventToInject.Name),
		reflect.ValueOf(eventToInject.Type),
		reflect.ValueOf(eventToInject.Message),
	})

	// Check if there was an error returned
	if len(callResults) > 0 && !callResults[0].IsNil() {
		return callResults[0].Interface().(error)
	}

	return nil
}

func (i *injector) ClearComponentFault(ctx context.Context, registry interface{}, componentClear *ComponentClear) error {
	// Use reflection to get the component from the registry
	registryValue := reflect.ValueOf(registry)
	getMethod := registryValue.MethodByName("Get")
	if !getMethod.IsValid() {
		return fmt.Errorf("registry does not have Get method")
	}

	// Call registry.Get(componentName)
	results := getMethod.Call([]reflect.Value{reflect.ValueOf(componentClear.Component)})
	if len(results) == 0 {
		return fmt.Errorf("registry Get method returned no results")
	}

	component := results[0]
	if component.IsNil() {
		return fmt.Errorf("component '%s' not found in registry", componentClear.Component)
	}

	// Get the underlying component
	componentValue := component.Elem()

	// Call the ClearFault method on the component
	method := componentValue.MethodByName("ClearFault")
	if !method.IsValid() {
		return fmt.Errorf("component %s does not have ClearFault method", componentClear.Component)
	}

	method.Call([]reflect.Value{})
	return nil
}

// Request is the request body for the inject-fault endpoint.
type Request struct {
	// XID is the XID to inject.
	XID *XIDToInject `json:"xid,omitempty"`

	// KernelMessage is the kernel message to inject.
	KernelMessage *pkgkmsgwriter.KernelMessage `json:"kernel_message,omitempty"`

	// ComponentError is the component error to inject.
	ComponentError *ComponentError `json:"component_error,omitempty"`

	// ComponentClear is the component fault to clear.
	ComponentClear *ComponentClear `json:"component_clear,omitempty"`

	// Event is the event to inject directly into a component's event store.
	Event *EventToInject `json:"event,omitempty"`
}

type XIDToInject struct {
	ID int `json:"id"`
}

type ComponentError struct {
	Component string `json:"component"`
	Message   string `json:"message"`
}

type ComponentClear struct {
	Component string `json:"component"`
}

type EventToInject struct {
	Component string `json:"component"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

var ErrNoFaultFound = errors.New("no fault injection entry found")

func (r *Request) Validate() error {
	switch {
	case r.XID != nil:
		if r.XID.ID == 0 {
			return ErrNoFaultFound
		}

		msg := componentsnvidiaxid.GetMessageToInject(r.XID.ID)
		r.KernelMessage = &pkgkmsgwriter.KernelMessage{
			Priority: pkgkmsgwriter.ConvertKernelMessagePriority(msg.Priority),
			Message:  msg.Message,
		}
		r.XID = nil

		// fall through to a subsequent case to call Validate()
		fallthrough

	case r.KernelMessage != nil:
		return r.KernelMessage.Validate()

	case r.ComponentError != nil:
		if r.ComponentError.Component == "" {
			return errors.New("component name is required for component error injection")
		}
		if r.ComponentError.Message == "" {
			r.ComponentError.Message = "Injected error for testing"
		}
		return nil

	case r.ComponentClear != nil:
		if r.ComponentClear.Component == "" {
			return errors.New("component name is required for component fault clearing")
		}
		return nil

	case r.Event != nil:
		if r.Event.Component == "" {
			return errors.New("component name is required for event injection")
		}
		if r.Event.Name == "" {
			r.Event.Name = "test_injected_event"
		}
		if r.Event.Type == "" {
			r.Event.Type = "Critical"
		}
		if r.Event.Message == "" {
			r.Event.Message = "Injected event for testing"
		}
		return nil

	default:
		return ErrNoFaultFound
	}
}
