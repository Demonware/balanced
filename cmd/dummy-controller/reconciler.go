package main

import (
	"github.com/golang/glog"
	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	coreinformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

type DummyReconciler struct {
	serviceInformer cache.SharedIndexInformer
	eventRecorder   record.EventRecorder
	sm              v1alpha1.ServiceManager
}

func NewDummyReconciler(s coreinformer.ServiceInformer, eventRecorder record.EventRecorder, sm *v1alpha1.MapServiceManager) *DummyReconciler {
	return &DummyReconciler{
		serviceInformer: s.Informer(),
		eventRecorder:   eventRecorder,
		sm:              sm,
	}
}

func (d *DummyReconciler) OnChangeServiceHandler(prevSvc *v1alpha1.Service, svc *v1alpha1.Service) {
	glog.Infof("DummyReconciler.OnChangeServiceHandler called")
	if prevSvc != nil {
		if svc != nil {
			// update case
			d.updateServiceHandler(prevSvc, svc)
			return
		}
		d.deleteServiceHandler(prevSvc)
		return
	}
	d.addServiceHandler(svc)
}

func (d *DummyReconciler) addServiceHandler(svc *v1alpha1.Service) {
	glog.Infof("DummyReconciler.addServiceHandler called: %s", svc.ID())
}

func (d *DummyReconciler) updateServiceHandler(prevSvc *v1alpha1.Service, svc *v1alpha1.Service) {
	glog.Infof("DummyReconciler.updateServiceHandler called: %s", svc.ID())
}

func (d *DummyReconciler) deleteServiceHandler(prevSvc *v1alpha1.Service) {
	glog.Infof("DummyReconciler.deleteServiceHandler called: %s", prevSvc.ID())
}

func (d *DummyReconciler) OnChangeEndpointHandler(prevE *v1alpha1.Endpoint, e *v1alpha1.Endpoint) {
	glog.Infof("DummyReconciler.OnChangeEndpointHandler called")
	if prevE != nil {
		if e != nil {
			d.updateEndpointHandler(prevE, e) // update case
		} else {
			d.deleteEndpointHandler(prevE) // delete case
		}
	} else {
		d.addEndpointHandler(e) // insert case
	}
}

func (d *DummyReconciler) addEndpointHandler(e *v1alpha1.Endpoint) {
	glog.Infof("DummyReconciler.addEndpointHandler called: %s (svc: %s)", e.ID(), e.Service().ID())
}

func (d *DummyReconciler) updateEndpointHandler(prevE *v1alpha1.Endpoint, e *v1alpha1.Endpoint) {
	glog.Infof("DummyReconciler.updateEndpointHandler called: %s (svc: %s)", e.ID(), e.Service().ID())
}

func (d *DummyReconciler) deleteEndpointHandler(prevE *v1alpha1.Endpoint) {
	glog.Infof("DummyReconciler.deleteEndpointHandler called: %s (svc: %s)", prevE.ID(), prevE.Service().ID())
}

func (d *DummyReconciler) PostSyncHandler() error {
	glog.Infof("DummyReconciler.PostSyncHandler called")
	return nil
}
