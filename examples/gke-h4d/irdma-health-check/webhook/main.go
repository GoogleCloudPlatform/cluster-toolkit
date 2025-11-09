package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

type WebhookServer struct {
	server *http.Server
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (whsvr *WebhookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = whsvr.mutate(&ar)
	}

	admissionReview := v1beta1.AdmissionReview{
		Response: admissionResponse,
	}
	if admissionResponse != nil {
		admissionReview.Response.UID = ar.Request.UID
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	if _, err := w.Write(resp); err != nil {
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func (whsvr *WebhookServer) mutate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request
	pod := corev1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if val, ok := pod.Labels["irdma-health-check"]; !ok || val != "true" {
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	var patches []patchOperation
	patches = append(patches, addInitContainer(pod, pod.Spec.InitContainers)...)

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

func addInitContainer(pod corev1.Pod, initContainers []corev1.Container) (patches []patchOperation) {
	// The full URI of the image built and pushed.
	imageURI := "us-central1-docker.pkg.dev/MY-GCP-PROJECT/h4d/irdma-health-check:v1.0.0"
	initContainer := corev1.Container{
		Name:  "rdma-health-check",
		Image: imageURI,
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"IPC_LOCK", "NET_ADMIN"},
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				"networking.gke.io.networks/rdma-0": resource.MustParse("1"),
			},
			Requests: corev1.ResourceList{
				"networking.gke.io.networks/rdma-0": resource.MustParse("1"),
			},
		},
	}

	// Check if initContainers is nil or empty
	if len(initContainers) == 0 {
		patches = append(patches, patchOperation{
			Op:    "add",
			Path:  "/spec/initContainers",
			Value: []corev1.Container{initContainer},
		})
	} else {
		patches = append(patches, patchOperation{
			Op:    "add",
			Path:  "/spec/initContainers/-",
			Value: initContainer,
		})
	}
	return patches
}

func main() {
	http.HandleFunc("/mutate", func(w http.ResponseWriter, r *http.Request) {
		whsvr := &WebhookServer{}
		whsvr.serve(w, r)
	})

	certPath := "/etc/webhook/certs/tls.crt"
	keyPath := "/etc/webhook/certs/tls.key"

	fmt.Println("Listening on :8443")
	if err := http.ListenAndServeTLS(":8443", certPath, keyPath, nil); err != nil {
		panic(err)
	}
}
