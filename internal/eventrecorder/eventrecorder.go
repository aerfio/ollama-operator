package eventrecorder

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
)

type EventRecorder struct {
	recorder events.EventRecorder
	obj      runtime.Object
}

func New(recorder events.EventRecorder, obj runtime.Object) *EventRecorder {
	return &EventRecorder{
		recorder: recorder,
		obj:      obj,
	}
}

func (e *EventRecorder) NormalEvent(action, reason, message string) {
	e.recorder.Eventf(e.obj, nil, corev1.EventTypeNormal, reason, action, message)
}

func (e *EventRecorder) NormalEventf(action, reason, format string, args ...any) {
	e.recorder.Eventf(e.obj, nil, corev1.EventTypeNormal, reason, action, format, args...)
}

func (e *EventRecorder) WarningEvent(action, reason, message string) {
	e.recorder.Eventf(e.obj, nil, corev1.EventTypeWarning, reason, action, message)
}

func (e *EventRecorder) WarningEventf(action, reason, format string, args ...any) {
	e.recorder.Eventf(e.obj, nil, corev1.EventTypeWarning, reason, action, format, args...)
}
