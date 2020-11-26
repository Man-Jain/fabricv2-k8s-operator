/*


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

package controllers

import (
	"context"
	"encoding/base64"
	"strconv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	fabricv1alpha1 "github.com/Man-Jain/fabricv2-k8s-operator/api/v1alpha1"
)

// CAReconciler reconciles a CA object
type CAReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabric.hyperledger.org,resources=cas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabric.hyperledger.org,resources=cas/status,verbs=get;update;patch

func (r *CAReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	reqLogger := r.Log.WithValues("ca", req.NamespacedName)

	instance := &fabricv1alpha1.CA{}
	err := r.Get(ctx, req.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("CA resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get CA.")
		return ctrl.Result{}, err
	}

	// secretID := req.Name + "-secret"
	foundSecret := &corev1.Secret{}
	secret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: instance.Name + "-secret", Namespace: instance.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) && !errors.IsAlreadyExists(err) {
		secret = r.newSecretForCR(instance, req)
		err = r.Create(ctx, secret)
		if err != nil {
			reqLogger.Error(err, "Failed to retrieve Fabric CA secrets")
			return ctrl.Result{}, err
		}
		// When we reach here, it means that we have created the secret successfully
		// and ready to do more
	}

	// secretID := req.Name + "-secret"
	foundConfigMap := &corev1.ConfigMap{}
	configMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: instance.Name + "-configfilemap", Namespace: instance.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) && !errors.IsAlreadyExists(err) {
		configMap = r.newConfigMapForCR(instance, req)
		err = r.Create(ctx, configMap)
		if err != nil {
			reqLogger.Error(err, "Failed to retrieve Fabric CA secrets")
			return ctrl.Result{}, err
		}
		// When we reach here, it means that we have created the secret successfully
		// and ready to do more
	}

	foundService := &corev1.Service{}
	err = r.Get(ctx, req.NamespacedName, foundService)
	if err != nil && errors.IsNotFound(err) && !errors.IsAlreadyExists(err) {
		// Define a new Service object
		service := r.newServiceForCR(instance, req)
		reqLogger.Info("Creating a new service.", "Service.Namespace", service.Namespace,
			"Service.Name", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			reqLogger.Error(err, "Failed to create new service for CA.", "Service.Namespace",
				service.Namespace, "Service.Name", service.Name)
			return ctrl.Result{}, err
		}
		r.Get(ctx, req.NamespacedName, foundService)
	} else if err != nil {
		reqLogger.Error(err, "Failed to get CA service.")
		return ctrl.Result{}, err
	}

	if len(foundService.Spec.Ports) == 0 {
		// if no ports yet, we need to wait.
		return ctrl.Result{Requeue: true}, nil
	}

	if instance.Status.AccessPoint == "" {
		if foundService.Spec.Ports[0].NodePort > 0 {
			reqLogger.Info("The service port has been found", "Service port", foundService.Spec.Ports[0].NodePort)

			instance.Status.AccessPoint = req.Name + ":" +
				strconv.FormatInt(int64(foundService.Spec.Ports[0].Port), 10)
			instance.Status.ExternalPort = int(foundService.Spec.Ports[0].NodePort)
			err = r.Client.Status().Update(context.TODO(), instance)
			if err != nil {
				reqLogger.Error(err, "Failed to update Peer status", "Fabric Peer namespace",
					instance.Namespace, "Fabric Peer Name", instance.Name)
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	foundSTS := &appsv1.StatefulSet{}
	err = r.Get(ctx, req.NamespacedName, foundSTS)
	if err != nil && errors.IsNotFound(err) && !errors.IsAlreadyExists(err) {
		// Define a new StatefulSet object
		sts := r.newSTSForCR(instance, secret, req)
		reqLogger.Info("Creating a new set.", "StatefulSet.Namespace", sts.Namespace,
			"StatefulSet.Name", sts.Name)
		err = r.Create(ctx, sts)
		if err != nil && !errors.IsAlreadyExists(err) {
			reqLogger.Error(err, "Failed creating new statefulset for CA.", "StatefulSet.Namespace",
				sts.Namespace, "StatefulSet.Name", sts.Name)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		reqLogger.Error(err, "Failed to get CA StatefulSet.")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CAReconciler) newSecretForCR(cr *fabricv1alpha1.CA, req ctrl.Request) *corev1.Secret {
	secret := &corev1.Secret{}
	secret.Name = req.Name + "-secret"
	secret.Type = "Opaque"
	secret.Namespace = req.Namespace
	secret.Data = make(map[string][]byte)

	if cr.Spec.Certs != nil {
		secret.Data["cert"], _ = base64.StdEncoding.DecodeString(cr.Spec.Certs.Cert)
		secret.Data["key"], _ = base64.StdEncoding.DecodeString(cr.Spec.Certs.Key)
		secret.Data["tlsCert"], _ = base64.StdEncoding.DecodeString(cr.Spec.Certs.TLSCert)
		secret.Data["tlsKey"], _ = base64.StdEncoding.DecodeString(cr.Spec.Certs.TLSKey)
	}

	controllerutil.SetControllerReference(cr, secret, r.Scheme)

	return secret
}

func (r *CAReconciler) newConfigMapForCR(cr *fabricv1alpha1.CA, req ctrl.Request) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{}
	configMap.ObjectMeta.Name = req.Name + "-configfilemap"
	configMap.ObjectMeta.Namespace = req.Namespace

	configData := make(map[string]string)
	configString, _ := base64.StdEncoding.DecodeString(cr.Spec.Config)
	configData["fabric-ca-server-config"] = string(configString)

	configMap.Data = configData

	controllerutil.SetControllerReference(cr, configMap, r.Scheme)

	return configMap
}

// newServiceForCR returns a fabric CA service with the same name/namespace as the cr
func (r *CAReconciler) newServiceForCR(cr *fabricv1alpha1.CA, req ctrl.Request) *corev1.Service {
	service := &corev1.Service{}
	service.Name = req.Name
	service.Namespace = req.Namespace
	service.Spec.Type = "NodePort"

	service.Spec.Ports = []corev1.ServicePort{
		corev1.ServicePort{
			Name:       "main",
			Port:       int32(cr.Spec.Ports[0]),
			TargetPort: intstr.FromInt(cr.Spec.Ports[0]),
			Protocol:   "TCP",
		},
	}

	service.Spec.Selector = map[string]string{
		"name": service.Name,
	}

	service.Labels = map[string]string{
		"name": service.Name,
	}

	controllerutil.SetControllerReference(cr, service, r.Scheme)
	return service
}

func (r *CAReconciler) newSTSForCR(cr *fabricv1alpha1.CA, secret *corev1.Secret, req ctrl.Request) *appsv1.StatefulSet {
	sts := &appsv1.StatefulSet{}
	sts.Name = req.Name
	sts.Namespace = req.Namespace
	sts.Spec.ServiceName = sts.Name

	sts.Spec.VolumeClaimTemplates = append(sts.Spec.VolumeClaimTemplates, corev1.PersistentVolumeClaim{})
	sts.Spec.VolumeClaimTemplates[0].ObjectMeta.Name = req.Name
	sts.Spec.VolumeClaimTemplates[0].ObjectMeta.Namespace = req.Namespace
	sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = &cr.Spec.StorageClass
	sts.Spec.VolumeClaimTemplates[0].Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests = make(map[corev1.ResourceName]resource.Quantity)
	sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests["storage"] = resource.MustParse(cr.Spec.StorageSize)

	sts.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"name": sts.Name,
		},
	}

	sts.Spec.Template.Labels = map[string]string{
		"name": sts.Name,
	}

	sts.Spec.Template.Spec.Containers = append(sts.Spec.Template.Spec.Containers, corev1.Container{})
	sts.Spec.Template.Spec.Containers[0].Name = req.Name
	// sts.Spec.Template.Spec.Containers[0].WorkingDir = "/opt/gopath/src/github.com/hyperledger/fabric"
	sts.Spec.Template.Spec.Containers[0].Image = cr.Spec.Image
	sts.Spec.Template.Spec.Containers[0].ImagePullPolicy = "Always"
	sts.Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh", "-c", "fabric-ca-server start -b admin:adminpw -d"}

	// sts.Spec.Template.Spec.Containers[0].Env[1].Value = req.Name
	for _, configParam := range cr.Spec.ConfigParams {
		sts.Spec.Template.Spec.Containers[0].Env =
			append(sts.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
				Name: configParam.Name, Value: configParam.Value,
			})
	}

	if cr.Spec.Certs != nil {
		sts.Spec.Template.Spec.Containers[0].Env =
			append(sts.Spec.Template.Spec.Containers[0].Env,
				corev1.EnvVar{Name: "FABRIC_CA_SERVER_CA_KEYFILE", Value: "/certs/key"},
				corev1.EnvVar{Name: "FABRIC_CA_SERVER_CA_CERTFILE", Value: "/certs/cert"},
				corev1.EnvVar{Name: "FABRIC_CA_SERVER_TLS_KEYFILE", Value: "/certs/tlsKey"},
				corev1.EnvVar{Name: "FABRIC_CA_SERVER_TLS_CERTFILE", Value: "/certs/tlsCert"},
				corev1.EnvVar{Name: "FABRIC_CA_HOME", Value: "/etc/hyperledger/fabric-ca-server-config"},
			)
	}

	configVolume := corev1.Volume{
		Name: req.Name + "-configfile-volume",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: req.Name + "-configfilemap",
				},
			},
		},
	}

	volumeCreds := corev1.Volume{
		Name: req.Name + "-creds-volume",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Name + "-secret",
			},
		},
	}

	sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, configVolume, volumeCreds)

	configVolumeMount := corev1.VolumeMount{
		Name:      req.Name + "-configfile-volume",
		MountPath: "/etc/hyperledger/fabric-ca-server-config",
	}

	credsVolumeMount := corev1.VolumeMount{
		Name:      req.Name + "-creds-volume",
		MountPath: "/certs",
	}

	sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(sts.Spec.Template.Spec.Containers[0].VolumeMounts, configVolumeMount, credsVolumeMount)

	controllerutil.SetControllerReference(cr, sts, r.Scheme)

	return sts
}

func (r *CAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricv1alpha1.CA{}).
		Complete(r)
}
