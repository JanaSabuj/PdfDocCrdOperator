/*
Copyright 2023.

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
	"encoding/base64"
	"fmt"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	customtoolsv1 "github.com/JanaSabuj/PdfDocCrdOperator/api/v1"
)

// PdfDocReconciler reconciles a PdfDoc object
type PdfDocReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=customtools.janasabuj.github.io,resources=pdfdocs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=customtools.janasabuj.github.io,resources=pdfdocs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=customtools.janasabuj.github.io,resources=pdfdocs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PdfDoc object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *PdfDocReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)
	lg.WithValues("PdfDoc", req.NamespacedName)

	// TODO(user): your logic here
	// get the PdfDoc
	var pdfDoc customtoolsv1.PdfDoc
	if err := r.Get(ctx, req.NamespacedName, &pdfDoc); err != nil {
		lg.Error(err, "Unable to fetch PdfDocument")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// create the JobSpec
	jobSpec, err := r.CreateJobSpec(pdfDoc)
	if err != nil {
		lg.Error(err, "failed to create the desired Job Spec")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// create the Job
	if err := r.Create(ctx, &jobSpec); err != nil {
		lg.Error(err, "Unable to create Job")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PdfDocReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&customtoolsv1.PdfDoc{}).
		Complete(r)
}

func (r *PdfDocReconciler) CreateJobSpec(doc customtoolsv1.PdfDoc) (batchv1.Job, error) {

	fmt.Println(doc.Spec.DocName, doc.Spec.RawText, "being transformed....")
	encodedText := base64.StdEncoding.EncodeToString([]byte(doc.Spec.RawText))
	docName := doc.Spec.DocName

	// init1 - base64 encode the data and dump to volume
	initContainer1 := corev1.Container{
		Name:    "text-to-md",
		Image:   "alpine",
		Command: []string{"/bin/sh"},
		Args: []string{
			"-c",
			fmt.Sprintf("echo %s | base64 -d >> /data/%s.md", encodedText, docName),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "dumpbox",
				MountPath: "/data",
			},
		},
	}

	// 2nd init container - decode the dumped text and convert to pdf
	initContainer2 := corev1.Container{
		Name:    "md-to-pdf",
		Image:   "auchida/pandoc",
		Command: []string{"/bin/sh"},
		Args: []string{
			"-c",
			fmt.Sprintf("pandoc -V documentclass=ltjsarticle --pdf-engine=lualatex -o /opt/docs/docs.pdf /opt/docs/%s.md", docName),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "dumpbox",
				MountPath: "/opt/docs",
			},
		},
	}

	// main container
	mainContainer := corev1.Container{
		Name:    "mainc",
		Image:   "alpine",
		Command: []string{"/bin/sh", "-c", "sleep 3600"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "dumpbox",
				MountPath: "/data",
			},
		},
	}

	// volume
	volume := corev1.Volume{
		Name: "dumpbox",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	// job
	job := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      doc.Spec.DocName + "-job",
			Namespace: doc.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{initContainer1, initContainer2},
					Containers:     []corev1.Container{mainContainer},
					Volumes:        []corev1.Volume{volume},
					RestartPolicy:  "OnFailure",
				},
			},
		},
	}

	return job, nil
}
