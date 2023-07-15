/*
Copyright 2023 Ritesh Mahajan.

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
	"fmt"
	"time"

	apiv1alpha1 "github.com/riteshkkdfkd/Chronos/api/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log.WithName("controller_scaler")

// ScalerReconciler reconciles a Scaler object
type ScalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=api.zeta.in,resources=scalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=api.zeta.in,resources=scalers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=api.zeta.in,resources=scalers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Scaler object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *ScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log := logger.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	log.Info("Reconcile called")
	scaler := &apiv1alpha1.Scaler{}

	err := r.Get(ctx, req.NamespacedName, scaler)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Scaler resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed")
		return ctrl.Result{}, err
	}
	startTime := scaler.Spec.Start
	endTime := scaler.Spec.End

	Stime, err := time.Parse(time.TimeOnly, startTime)
	Etime, err := time.Parse(time.TimeOnly, endTime)
	if err != nil {
		fmt.Println("Error parsing timestamp:", err)
		return ctrl.Result{}, err
	}
	// current time in UTC
	currentTime := time.Now().UTC()
	log.Info(fmt.Sprintf("current time : %v\n", currentTime))

	Stime = Stime.UTC()
	Stime_hour := Stime.Hour()
	Stime_min := Stime.Minute()
	Stime_sec := Stime.Second()
	Stime = time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), Stime_hour, Stime_min, Stime_sec, currentTime.Nanosecond(), currentTime.Location())
	log.Info(fmt.Sprintf("Stime time : %v\n", Stime))

	Etime = Etime.UTC()
	Etime_hour := Etime.Hour()
	Etime_min := Etime.Minute()
	Etime_sec := Etime.Second()
	Etime = time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), Etime_hour, Etime_min, Etime_sec, currentTime.Nanosecond(), currentTime.Location())
	log.Info(fmt.Sprintf("Etime time : %v\n", Etime))

	Sstatus := currentTime.Compare(Stime)
	log.Info(fmt.Sprintf("Sstatus time : %v\n", Sstatus))
	Estatus := currentTime.Compare(Etime)
	log.Info(fmt.Sprintf("Estatus time : %v\n", Estatus))

	if Sstatus == 1 && Estatus == -1 {
		if scaler.Spec.Deployment != nil {
			if err = scaleDeployment(scaler, r, ctx); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			log.Info(fmt.Sprintf("This hpa block %d", 1))
			if err = scalerHpa(scaler, r, ctx); err != nil {
				return ctrl.Result{}, err
			}
		}

	}

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.Scaler{}).
		Complete(r)
}

func scaleDeployment(scaler *apiv1alpha1.Scaler, r *ScalerReconciler, ctx context.Context) error {

	for _, deploy := range scaler.Spec.Deployment {
		dep := &v1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{
			Namespace: deploy.Namespace,
			Name:      deploy.Name,
		}, dep)

		if err != nil {
			return err
		}
		replica := &deploy.Replica
		if replica != dep.Spec.Replicas {
			scaler.Status.OriginalReplica = *dep.Spec.Replicas
			*dep.Spec.Replicas = *replica

			err := r.Update(ctx, dep)
			if err != nil {
				scaler.Status.Status = apiv1alpha1.FAILED
				return err
			}
			scaler.Status.Status = apiv1alpha1.SUCCESS

			err = r.Status().Update(ctx, scaler)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func scalerHpa(scaler *apiv1alpha1.Scaler, r *ScalerReconciler, ctx context.Context) error {

	for _, deploy := range scaler.Spec.HPA {
		hpa := &v2.HorizontalPodAutoscaler{}
		err := r.Get(ctx, types.NamespacedName{
			Namespace: deploy.Namespace,
			Name:      deploy.Name,
		}, hpa)

		if err != nil {
			return err
		}
		replica := &deploy.Minreplica
		if replica != hpa.Spec.MinReplicas {
			scaler.Status.OriginalReplica = *hpa.Spec.MinReplicas
			*hpa.Spec.MinReplicas = *replica
			hpa.Spec.MaxReplicas = deploy.Maxreplica

			err := r.Update(ctx, hpa)
			if err != nil {
				scaler.Status.Status = apiv1alpha1.FAILED
				return err
			}
			scaler.Status.Status = apiv1alpha1.SUCCESS
			scaler.Status.OriginalMinReplica = *hpa.Spec.MinReplicas
			scaler.Status.OriginalMaxReplica = hpa.Spec.MaxReplicas

			err = r.Status().Update(ctx, scaler)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
