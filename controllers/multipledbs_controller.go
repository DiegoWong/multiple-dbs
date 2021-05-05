/*
Copyright 2021.

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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbv1alpha1 "github.com/DiegoWong/multiple-dbs.git/api/v1alpha1"
)

// MultipleDbsReconciler reconciles a MultipleDbs object
type MultipleDbsReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=db.foo.com,resources=multipledbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=db.foo.com,resources=multipledbs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=db.foo.com,resources=multipledbs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MultipleDbs object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *MultipleDbsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("multiple dbs", req.NamespacedName)

	multipledbs := &dbv1alpha1.MultipleDbs{}
	err := r.Get(ctx, req.NamespacedName, multipledbs)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get multiple dbs")
		return ctrl.Result{}, err
	}

	hasImage := make(map[string]bool)

	for _, db := range multipledbs.Spec.Dbs {
		// Check if the deployment already exists, if not create a new one
		found := &appsv1.Deployment{}
		deploymentName := multipledbs.Name + "-" + db.Image
		err = r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: multipledbs.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			// Define a new deployment
			dep := r.dbDeployment(multipledbs, db.Replicas, db.Image)
			log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", deploymentName)
			err = r.Create(ctx, dep)
			if err != nil {
				log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", deploymentName)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}
		size := db.Replicas
		if *found.Spec.Replicas != size {
			found.Spec.Replicas = &size
			err = r.Update(ctx, found)
			if err != nil {
				log.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
				return ctrl.Result{}, err
			}
			// Spec updated - return and requeue
			return ctrl.Result{Requeue: true}, nil
		}

		hasImage[db.Image] = true
	}

	deployList := &appsv1.DeploymentList{}
	multipledbsLabel := client.MatchingLabels{"multipledbs": multipledbs.Name}
	err = r.List(ctx, deployList, multipledbsLabel)

	for _, deploy := range deployList.Items {
		_, found := hasImage[deploy.Labels["image"]]
		log.Info("Delete Deployment", "Deployment.Namespace", deploy.Namespace, "Deployment.Name", deploy.Name)
		if !found {
			err = r.Delete(ctx, &deploy)
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultipleDbsReconciler) dbDeployment(m *dbv1alpha1.MultipleDbs, replicas int32, image string) *appsv1.Deployment {
	ls := labels(m.Name)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + "-" + image,
			Namespace: m.Namespace,
			Labels:    map[string]string{"multipledbs": m.Name, "image": image},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: image,
						Name:  "container-" + image,
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8080,
							Name:          "db-" + image,
						}},
					}},
				},
			},
		},
	}
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

func labels(name string) map[string]string {
	return map[string]string{"app": "multipledbs", "multipledbs_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// SetupWithManager sets up the controller with the Manager.
func (r *MultipleDbsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.MultipleDbs{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
