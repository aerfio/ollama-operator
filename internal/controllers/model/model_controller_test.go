package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func Test_isStatefulSetReady(t *testing.T) {
	tests := []struct {
		name        string
		sts         *appsv1.StatefulSet
		ready       bool
		msgContains string
	}{
		{
			name: "sts not ready - too few ready replicas",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 123,
				},
				Spec: appsv1.StatefulSetSpec{
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
					},
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 123,
					Replicas:           3,
					ReadyReplicas:      1,
				},
			},
			ready:       false,
			msgContains: "Waiting for 2 pods",
		},
		{
			name: "sts ready",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 123,
				},
				Spec: appsv1.StatefulSetSpec{
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
					},
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 123,
					Replicas:           3,
					ReadyReplicas:      3,
				},
			},
			ready:       true,
			msgContains: "rolling update complete",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ready, err := isStatefulSetReady(tt.sts)
			require.NoError(t, err) // should never err
			assert.Equal(t, tt.ready, ready, msg)
			if tt.msgContains != "" {
				assert.Contains(t, msg, tt.msgContains)
			}
		})
	}
}
