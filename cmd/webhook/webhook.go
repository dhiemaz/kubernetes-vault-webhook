package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	universalDeserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()

	ter = promauto.NewCounter(prometheus.CounterOpts{
		Name:      "errors_total",
		Help:      "the total amount of errors",
		Namespace: "vault_webhook",
	})

	mutated = promauto.NewCounter(prometheus.CounterOpts{
		Name:      "mutations_total",
		Help:      "the total amount of successfull mutations",
		Namespace: "vault_webhook",
	})
)

const (

	// this enables the webhook to mutuate the request
	// providing a vaultRoleKey is also not nil
	enableKey = "vault.mackers.com/enabled"

	// only required to override the default vault address
	vaultAddressKey = "vault.mackers.com/address"

	// this is the role to use on vault to grab the secrets
	vaultRoleKey = "vault.mackers.com/role"

	// this is used to inject stutus injected or failed
	vaultStatusKey = "vault.mackers.com/status"

	// if this is set to true.  Any failures to block the pod from starting by rejecting the admission review
	denyOnFailure = "vault.mackers.com/deny"
)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func serve(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		ter.Inc()
		return
	}

	if len(body) == 0 {
		log.Error("the request body is empty")
		http.Error(w, "the request body is empty", http.StatusBadRequest)
		ter.Inc()
		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		log.Errorf("the content-type is %s, but expect application/json", r.Header.Get("Content-Type"))
		http.Error(w, "invalid content-type", http.StatusUnsupportedMediaType)
		ter.Inc()
		return
	}

	request := v1beta1.AdmissionReview{}

	if _, _, err := universalDeserializer.Decode(body, nil, &request); err != nil {
		log.Errorf("could not deserialize request: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if request.Request == nil {
		log.Error("no admission review in request")
		ter.Inc()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	response := v1beta1.AdmissionReview{Response: &v1beta1.AdmissionResponse{UID: request.Request.UID}}

	var pod corev1.Pod
	if err := json.Unmarshal(request.Request.Object.Raw, &pod); err != nil {
		log.Errorf("Could not unmarshal raw object into a pod: %v", err)
		ter.Inc()
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	response.Response.Allowed = true
	required, err := mutationRequired(&pod)
	if err != nil {
		ter.Inc()
		log.Error("error parsing annotations: ", err)
	}

	if required {
		bytes, _ := createPatch(&pod)
		response.Response.Patch = bytes
	}

	reviewBytes, err := json.Marshal(response)

	if err != nil {
		log.Error(err)
		ter.Inc()
		http.Error(w, fmt.Sprintf("problem marshaling json: %v", err), http.StatusInternalServerError)
	}

	if _, err := w.Write(reviewBytes); err != nil {
		ter.Inc()
		log.Errorf("error writting response back to kubernetes: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
		return
	}
	mutated.Inc()
}

type mapping struct {
	env  string
	path string
	key  string
}

func getSecretPaths(p *corev1.Pod) []mapping {
	var m []mapping
	for i := range p.Spec.Containers {
		e := p.Spec.Containers[i].Env
		for _, k := range e {
			if strings.HasPrefix(k.Value, "vault:") {
				val := strings.Split(strings.TrimPrefix(k.Value, "vault:"), ":")
				m = append(m, mapping{
					env:  k.Name,
					key:  val[1],
					path: val[0],
				})
			}
		}
	}
	return m
}

func addCredentialVolume() (patch patchOperation) {
	volume := corev1.Volume{
		Name: "secrets",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: "Memory",
			},
		},
	}
	value := []corev1.Volume{volume}

	return patchOperation{
		Op:    "add",
		Path:  "/spec/volumes",
		Value: value,
	}
}

func updatePodSpec(p *corev1.Pod) (patch patchOperation) {

	vm := corev1.VolumeMount{
		Name:      "secrets",
		ReadOnly:  true,
		MountPath: "/var/run/secrets/vault",
	}

	for i := range p.Spec.Containers {
		c := &p.Spec.Containers[i]
		c.VolumeMounts = append(c.VolumeMounts, vm)
	}

	return patchOperation{
		Op:    "replace",
		Path:  "/spec/containers",
		Value: p.Spec.Containers,
	}
}

func buildInitCommand(m []mapping) []string {
	var cmd []string
	for _, p := range m {
		s := fmt.Sprintf("--secret=%s:%s:%s", p.env, p.path, p.key)
		cmd = append(cmd, s)
	}
	cmd = append(cmd, fmt.Sprintf("--address=http://vault:8200"))
	return cmd
}

func addInitContainer(p *corev1.Pod) (patch patchOperation) {

	cmd := buildInitCommand(getSecretPaths(p))

	req := corev1.ResourceList{
		"cpu":    resource.MustParse(cfg.InitContainerCPURequests),
		"memory": resource.MustParse(cfg.InitContainerMemoryRequests),
	}

	lim := corev1.ResourceList{
		"cpu":    resource.MustParse(cfg.InitContainerCPULimits),
		"memory": resource.MustParse(cfg.InitContainerMemoryLimits),
	}

	vault := corev1.Container{
		Name:            "vault-init",
		Image:           cfg.InitImage,
		ImagePullPolicy: "Always",

		Resources: corev1.ResourceRequirements{
			Requests: req,
			Limits:   lim,
		},
		VolumeMounts: []corev1.VolumeMount{
			corev1.VolumeMount{
				Name:      "secrets",
				MountPath: "/var/run/secrets/vault",
				ReadOnly:  false,
			},
		},
		Args: cmd,
	}

	p.Spec.InitContainers = append(p.Spec.InitContainers, vault)

	return patchOperation{
		Op:    "add",
		Path:  "/spec/initContainers",
		Value: p.Spec.InitContainers,
	}
}

func createPatch(p *corev1.Pod) ([]byte, error) {
	var patch []patchOperation
	patch = append(patch, addCredentialVolume())
	patch = append(patch, updatePodSpec(p))
	patch = append(patch, addInitContainer(p))
	return json.Marshal(patch)
}

func mutationRequired(p *corev1.Pod) (bool, error) {
	a := p.GetAnnotations()
	if _, ok := a[enableKey]; ok {
		enabled, err := strconv.ParseBool(a[enableKey])
		if err != nil {
			enabled = false
			ter.Inc()
			return enabled, err
		}
		if _, ok := a[vaultRoleKey]; !ok {
			e := fmt.Sprintf("annotation %s is required to enable mutation", vaultRoleKey)
			err = errors.New(e)
			return false, err
		}
		return enabled, nil
	}
	return false, nil
}
