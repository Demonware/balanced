package util

import (
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
)

func NewEventRecorder(client clientset.Interface, componentName string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.V(2).Infof)
	eventBroadcaster.StartRecordingToSink(
		&v1core.EventSinkImpl{
			Interface: v1core.New(
				client.CoreV1().RESTClient()).Events(""),
		},
	)
	return eventBroadcaster.NewRecorder(
		scheme.Scheme, v1.EventSource{Component: componentName})
}
