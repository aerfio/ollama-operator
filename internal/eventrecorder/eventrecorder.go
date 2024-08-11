package eventrecorder

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

type EventRecorder struct {
	recorder record.EventRecorder
	obj      runtime.Object
}

func New(recorder record.EventRecorder, obj runtime.Object) *EventRecorder {
	return &EventRecorder{
		recorder: recorder,
		obj:      obj,
	}
}

func (e *EventRecorder) NormalEvent(reason, message string) {
	e.recorder.Event(e.obj, corev1.EventTypeNormal, reason, message)
}

func (e *EventRecorder) NormalEventf(reason, format string, args ...any) {
	e.recorder.Eventf(e.obj, corev1.EventTypeNormal, reason, format, args...)
}

func (e *EventRecorder) WarningEvent(reason, message string) {
	e.recorder.Event(e.obj, corev1.EventTypeWarning, reason, message)
}

func (e *EventRecorder) WarningEventf(reason, format string, args ...any) {
	e.recorder.Eventf(e.obj, corev1.EventTypeWarning, reason, format, args...)
}
