package webhook

import (
	"context"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"storageclass-accessor/client/apis/accessor/v1alpha1"
)

var reviewResponse = &admissionv1.AdmissionResponse{
	Allowed: true,
	Result:  &metav1.Status{},
}

func admitPVC(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	klog.Info("admitting pvc")

	if !(ar.Request.Operation == admissionv1.Delete || ar.Request.Operation == admissionv1.Create) {
		return reviewResponse
	}

	raw := ar.Request.Object.Raw

	var newPVC *corev1.PersistentVolumeClaim

	switch ar.Request.Operation {
	case admissionv1.Create:
		deserializer := codecs.UniversalDeserializer()
		pvc := &corev1.PersistentVolumeClaim{}
		obj, _, err := deserializer.Decode(raw, nil, pvc)
		if err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		var ok bool
		newPVC, ok = obj.(*corev1.PersistentVolumeClaim)
		if !ok {
			klog.Error("obj can't exchange to pvc object")
			return toV1AdmissionResponse(err)
		}
	case admissionv1.Delete:
		pvcInfo := types.NamespacedName{
			Namespace: ar.Request.Namespace,
			Name:      ar.Request.Name,
		}
		cli, err := client.New(config.GetConfigOrDie(), client.Options{})
		if err != nil {
			return toV1AdmissionResponse(err)
		}
		targetPVC := &corev1.PersistentVolumeClaim{}
		err = cli.Get(context.Background(), pvcInfo, targetPVC)
		if err != nil {
			klog.Error("get target Delete PVC from client failed, err:", err)
			return toV1AdmissionResponse(err)
		}
		newPVC = targetPVC
	}
	return decidePVCV1(newPVC)
}

func decidePVCV1(pvc *corev1.PersistentVolumeClaim) *admissionv1.AdmissionResponse {
	// get config
	cfg, err := config.GetConfig()
	if err != nil {
		return toV1AdmissionResponse(err)
	}
	var cli client.Client
	opts := client.Options{}
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	opts.Scheme = scheme
	cli, err = client.New(cfg, opts)
	if err != nil {
		return toV1AdmissionResponse(err)
	}
	accessor := &v1alpha1.Accessor{}

	err = cli.Get(context.Background(), types.NamespacedName{Namespace: "", Name: *pvc.Spec.StorageClassName + "-accessor"}, accessor)
	if err != nil {
		//TODO If not found , pass or not?
		return toV1AdmissionResponse(err)
	}

	if err = validateNameSpace("persistentVolumeClaim", pvc.Name, pvc.Namespace, accessor); err != nil {
		return toV1AdmissionResponse(err)
	}

	if err := validateWorkSpace("persistentVolumeClaim", pvc.Name, pvc.Namespace, accessor); err != nil {
		return toV1AdmissionResponse(err)
	}
	return reviewResponse
}
