/*
Copyright 2025.

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

package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kubernellov1beta1 "github.com/ludusrusso/kubernello-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kubernello.kubernello.eu,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubernello.kubernello.eu,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubernello.kubernello.eu,resources=applications/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Application object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	application := &kubernellov1beta1.Application{}
	err := r.Get(ctx, req.NamespacedName, application)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	err = r.reconcileDeployment(ctx, application)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.reconcileService(ctx, application)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.reconcileIngress(ctx, application)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubernellov1beta1.Application{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}

func (r *ApplicationReconciler) reconcileDeployment(ctx context.Context, application *kubernellov1beta1.Application) error {
	deployment := &appsv1.Deployment{}
	name := deploymentName(application)
	namespace := application.Namespace

	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deployment)
	if err == nil {
		deployment.Spec = buildDeploymentSpecs(application)
		return r.Update(ctx, deployment)
	} else if !errors.IsNotFound(err) {
		return err
	}

	deployment, err = r.buildDeployment(application)
	if err != nil {
		return err
	}

	err = r.Create(ctx, deployment)
	if err != nil {
		return err
	}

	return nil
}

func (r *ApplicationReconciler) reconcileService(ctx context.Context, application *kubernellov1beta1.Application) error {
	service := &corev1.Service{}
	name := serviceName(application)
	namespace := application.Namespace

	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, service)
	if err == nil {
		service.Spec = buildServiceSpecs(application)
		return r.Update(ctx, service)
	} else if !errors.IsNotFound(err) {
		return err
	}

	service, err = r.buildService(application)
	if err != nil {
		return err
	}

	return r.Create(ctx, service)
}

func (r *ApplicationReconciler) reconcileIngress(ctx context.Context, application *kubernellov1beta1.Application) error {
	ingress := &networkingv1.Ingress{}
	name := ingressName(application)
	namespace := application.Namespace

	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ingress)
	if err == nil {
		ingress.Spec = buildIngressSpecs(application)
		return r.Update(ctx, ingress)
	} else if !errors.IsNotFound(err) {
		return err
	}

	ingress, err = r.buildIngress(application)
	if err != nil {
		return err
	}

	return r.Create(ctx, ingress)
}

func deploymentName(application *kubernellov1beta1.Application) string {
	return application.Name + "-deployment"
}

func serviceName(application *kubernellov1beta1.Application) string {
	return application.Name + "-service"
}

func ingressName(application *kubernellov1beta1.Application) string {
	return application.Name + "-ingress"
}

func (r *ApplicationReconciler) buildDeployment(application *kubernellov1beta1.Application) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName(application),
			Namespace: application.Namespace,
		},
		Spec: buildDeploymentSpecs(application),
	}

	if err := controllerutil.SetControllerReference(application, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func buildDeploymentSpecs(application *kubernellov1beta1.Application) appsv1.DeploymentSpec {
	var repl int32 = 1
	lbs := makeLabels(application)

	return appsv1.DeploymentSpec{
		Replicas: &repl,
		Selector: &metav1.LabelSelector{
			MatchLabels: lbs,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: lbs,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  application.Name,
						Image: application.Spec.Image,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: int32(application.Spec.Port),
							},
						},
					},
				},
			},
		},
	}
}

func (r *ApplicationReconciler) buildService(application *kubernellov1beta1.Application) (*corev1.Service, error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName(application),
			Namespace: application.Namespace,
		},
		Spec: buildServiceSpecs(application),
	}

	if err := controllerutil.SetControllerReference(application, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func buildServiceSpecs(application *kubernellov1beta1.Application) corev1.ServiceSpec {
	lbls := makeLabels(application)

	return corev1.ServiceSpec{
		Selector: lbls,
		Ports: []corev1.ServicePort{
			{
				Port:       int32(application.Spec.Port),
				TargetPort: intstr.FromInt(application.Spec.Port),
			},
		},
	}
}

func (r *ApplicationReconciler) buildIngress(application *kubernellov1beta1.Application) (*networkingv1.Ingress, error) {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName(application),
			Namespace: application.Namespace,
		},
		Spec: buildIngressSpecs(application),
	}

	if err := controllerutil.SetControllerReference(application, ingress, r.Scheme); err != nil {
		return nil, err
	}

	return ingress, nil
}

func buildIngressSpecs(application *kubernellov1beta1.Application) networkingv1.IngressSpec {
	pathType := networkingv1.PathTypePrefix
	ingressClassName := "nginx"

	return networkingv1.IngressSpec{
		IngressClassName: &ingressClassName,
		Rules: []networkingv1.IngressRule{
			{
				Host: application.Spec.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: serviceName(application),
										Port: networkingv1.ServiceBackendPort{
											Number: int32(application.Spec.Port),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func makeLabels(application *kubernellov1beta1.Application) map[string]string {
	return map[string]string{
		"application": application.Name,
	}
}
