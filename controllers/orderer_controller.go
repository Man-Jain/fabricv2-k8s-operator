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
	"github.com/labstack/gommon/log"
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

// OrdererReconciler reconciles a Orderer object
type OrdererReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabric.hyperledger.org,resources=orderers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabric.hyperledger.org,resources=orderers/status,verbs=get;update;patch

func (r *OrdererReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	reqLogger := r.Log.WithValues("orderer", req.NamespacedName)

	instance := &fabricv1alpha1.Orderer{}
	err := r.Get(ctx, req.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Orderer resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Orderer.")
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
			reqLogger.Error(err, "Failed to retrieve Fabric Orderer secrets")
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
			reqLogger.Error(err, "Failed to create new service for Orderer.", "Service.Namespace",
				service.Namespace, "Service.Name", service.Name)
			return ctrl.Result{}, err
		}
		r.Get(ctx, req.NamespacedName, foundService)
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Orderer service.")
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
			reqLogger.Error(err, "Failed creating new statefulset for Orderer.", "StatefulSet.Namespace",
				sts.Namespace, "StatefulSet.Name", sts.Name)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Orderer StatefulSet.")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OrdererReconciler) newSecretForCR(cr *fabricv1alpha1.Orderer, req ctrl.Request) *corev1.Secret {
	secret := &corev1.Secret{}
	secret.Name = req.Name + "-secret"
	secret.Type = "Opaque"
	secret.Namespace = req.Namespace
	secret.Data = make(map[string][]byte)

	if cr.Spec.CaCerts == nil || len(cr.Spec.CaCerts) == 0 ||
		len(cr.Spec.KeyStore) == 0 || len(cr.Spec.SignCerts) == 0 ||
		cr.Spec.TLSCacerts == nil || len(cr.Spec.TLSCacerts) == 0 ||
		len(cr.Spec.TLS.TLSCert) == 0 || len(cr.Spec.TLS.TLSKey) == 0 {
		log.Error(errors.NewBadRequest("All entries under MSP and TLS in the request are required."),
			"Orderer creation", "Provide all certs under msp and tls in the request")
		return nil
	}

	for i, adminCert := range cr.Spec.AdminCerts {
		secret.Data["admincert"+strconv.Itoa(i)], _ = base64.StdEncoding.DecodeString(adminCert)
	}
	for i, caCert := range cr.Spec.CaCerts {
		secret.Data["cacert"+strconv.Itoa(i)], _ = base64.StdEncoding.DecodeString(caCert)
	}
	secret.Data["keystore"], _ = base64.StdEncoding.DecodeString(cr.Spec.KeyStore)
	secret.Data["signcert"], _ = base64.StdEncoding.DecodeString(cr.Spec.SignCerts)

	for i, tlsCacerts := range cr.Spec.TLSCacerts {
		secret.Data["tlscacert"+strconv.Itoa(i)], _ = base64.StdEncoding.DecodeString(tlsCacerts)
	}

	secret.Data["tlscert"], _ = base64.StdEncoding.DecodeString(cr.Spec.TLS.TLSCert)
	secret.Data["tlskey"], _ = base64.StdEncoding.DecodeString(cr.Spec.TLS.TLSKey)
	secret.Data["tlscacert"], _ = base64.StdEncoding.DecodeString(cr.Spec.TLS.TLSCaCert)
	secret.Data["config.yaml"], _ = base64.StdEncoding.DecodeString(cr.Spec.MSP.ConfigFile)

	controllerutil.SetControllerReference(cr, secret, r.Scheme)

	return secret
}

// newServiceForCR returns a fabric Orderer service with the same name/namespace as the cr
func (r *OrdererReconciler) newServiceForCR(cr *fabricv1alpha1.Orderer, req ctrl.Request) *corev1.Service {
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

// newStatefulSetForCR returns a fabric Orderer statefulset with the same name/namespace as the cr
func (r *OrdererReconciler) newSTSForCR(cr *fabricv1alpha1.Orderer, secret *corev1.Secret, req ctrl.Request) *appsv1.StatefulSet {
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
	sts.Spec.Template.Spec.Containers[0].WorkingDir = "/opt/gopath/src/github.com/hyperledger/fabric"
	sts.Spec.Template.Spec.Containers[0].Image = cr.Spec.Image
	sts.Spec.Template.Spec.Containers[0].ImagePullPolicy = "Always"
	sts.Spec.Template.Spec.Containers[0].Command = []string{"orderer"}

	for _, e := range cr.Spec.ConfigParams {
		sts.Spec.Template.Spec.Containers[0].Env = append(sts.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name: e.Name, Value: e.Value,
		})
	}

	sts.Spec.Template.Spec.Containers[0].Resources = cr.Spec.Resources

	mspVolume, tlsVolume := r.mountSecretsToSTS(cr, secret, req)

	genesisBlockVolume := corev1.Volume{
		Name: req.Name + "-genesis-block",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: cr.Spec.GenesisBlock,
			},
		},
	}

	sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, *mspVolume, *tlsVolume, genesisBlockVolume)

	mspVolumeMount := corev1.VolumeMount{
		Name:      req.Name + "-secrets-volume-msp",
		MountPath: "/var/hyperledger/orderer/msp",
		ReadOnly:  true,
	}

	tlsVolumeMount := corev1.VolumeMount{
		Name:      req.Name + "-secrets-volume-tls",
		MountPath: "/var/hyperledger/orderer/tls",
		ReadOnly:  true,
	}

	genesisBlockMount := corev1.VolumeMount{
		Name:      req.Name + "-genesis-block",
		MountPath: "/var/hyperledger/orderer/genesis.block",
	}

	sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(sts.Spec.Template.Spec.Containers[0].VolumeMounts, mspVolumeMount, tlsVolumeMount, genesisBlockMount)

	controllerutil.SetControllerReference(cr, sts, r.Scheme)

	return sts
}

func (r *OrdererReconciler) mountSecretsToSTS(cr *fabricv1alpha1.Orderer, secret *corev1.Secret, req ctrl.Request) (*corev1.Volume, *corev1.Volume) {
	volumeMSP := &corev1.Volume{
		Name: req.Name + "-secrets-volume-msp",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Name + "-secret",
			},
		},
	}
	itemsMSP := []corev1.KeyToPath{}

	// Make MSP Certs Paths to Volume
	for i := range cr.Spec.AdminCerts {
		keyPath := corev1.KeyToPath{
			Key:  "admincert" + strconv.Itoa(i),
			Path: "admincerts/admincert" + strconv.Itoa(i),
		}

		itemsMSP = append(itemsMSP, keyPath)
	}

	for i := range cr.Spec.CaCerts {
		keyPath := corev1.KeyToPath{
			Key:  "cacert" + strconv.Itoa(i),
			Path: "cacerts/cacert" + strconv.Itoa(i),
		}

		itemsMSP = append(itemsMSP, keyPath)
	}

	keyStoreKey := corev1.KeyToPath{
		Key:  "keystore",
		Path: "keystore/priv.key",
	}

	signCertKey := corev1.KeyToPath{
		Key:  "signcert",
		Path: "signcerts/signcert",
	}

	for i := range cr.Spec.TLSCacerts {
		keyPath := corev1.KeyToPath{
			Key:  "tlscacert" + strconv.Itoa(i),
			Path: "tlscacerts/tlscacert" + strconv.Itoa(i),
		}

		itemsMSP = append(itemsMSP, keyPath)
	}

	configKey := corev1.KeyToPath{
		Key:  "config.yaml",
		Path: "config.yaml",
	}

	itemsMSP = append(itemsMSP, keyStoreKey, signCertKey, configKey)

	volumeMSP.VolumeSource.Secret.Items = itemsMSP

	// Make TLS Certs Paths to Volume

	volumeTLS := &corev1.Volume{
		Name: req.Name + "-secrets-volume-tls",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Name + "-secret",
			},
		},
	}

	tlsCertKey := corev1.KeyToPath{
		Key:  "tlscert",
		Path: "server.crt",
	}

	tlsKey := corev1.KeyToPath{
		Key:  "tlskey",
		Path: "server.key",
	}

	tlsCaCert := corev1.KeyToPath{
		Key:  "tlscacert",
		Path: "ca.crt",
	}

	volumeTLS.VolumeSource.Secret.Items = append(volumeTLS.VolumeSource.Secret.Items, tlsCertKey, tlsKey, tlsCaCert)

	return volumeMSP, volumeTLS
}

func (r *OrdererReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricv1alpha1.Orderer{}).
		Complete(r)
}
