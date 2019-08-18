package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	fixtureType = metav1.TypeMeta{
		APIVersion: "admission.k8s.io/v1beta1",
		Kind:       "AdmissionReview",
	}
	fixtureObject = runtime.RawExtension{
		Object: &corev1.Pod{
			Spec: corev1.PodSpec{},
		},
	}
	fixtureResource = metav1.GroupVersionResource{
		Resource: "Pod",
		Version:  "v1",
	}
)

func TestGetSecretPaths(t *testing.T) {
	pod := fixtureObject.Object.(*corev1.Pod)
	pod.Spec = corev1.PodSpec{
		Containers: []corev1.Container{
			corev1.Container{
				Env: []corev1.EnvVar{
					corev1.EnvVar{
						Name:  "APP_PASSWORD1",
						Value: "vault:/secret/sql/pgpassword1",
					},
					corev1.EnvVar{
						Name:  "APP_PASSWORD2",
						Value: "vault:/secret/sql/pgpassword2",
					},
					corev1.EnvVar{
						Name:  "APP_PASSWORD3",
						Value: "vault:/secret/sql/pgpassword3",
					},
				},
			},
		},
	}
	paths := getSecretPaths(pod)

	expected := map[string]string{
		"APP_PASSWORD1": "secret/app/password1",
		"APP_PASSWORD2": "secret/app/password2",
		"APP_PASSWORD3": "secret/app/password3",
	}

	eq := reflect.DeepEqual(paths, expected)

	if !eq {
		exp := len(expected)
		got := len(paths)

		if exp != got {
			t.Errorf("expected %d vault paths but only got %d", exp, got)
		}

		for k, v := range expected {
			if _, ok := paths[k]; !ok {
				t.Errorf("expected key %s to exists with value %s", k, v)
			}
		}
	}
}

func TestMutationRequired(t *testing.T) {
	pod := fixtureObject.Object.(*corev1.Pod)
	tt := []struct {
		name        string
		annotations map[string]string
		err         string
		result      bool
	}{
		{name: "correct annotations", annotations: map[string]string{enableKey: "true", vaultRoleKey: "test"}, err: "", result: true},
		{name: "missing annotations", annotations: map[string]string{}, err: "", result: false},
	}

	for _, tc := range tt {
		pod.ObjectMeta = metav1.ObjectMeta{
			Annotations: tc.annotations,
		}

		mr, err := mutationRequired(pod)

		if err != nil {
			if err.Error() != tc.err {
				t.Errorf("expected error %s but got %s", tc.err, err)
			}
		}

		if mr != tc.result {
			t.Errorf("expected the method to return %v but got %v", tc.result, mr)
		}
	}
}

func TestHandler(t *testing.T) {
	tt := []struct {
		name         string
		responseCode int
		responseBody string
		err          string
		contentType  string
		a            interface{}
	}{
		{name: "correct request", responseCode: 200, responseBody: `{"response":{"uid":"1","allowed":true}}`, contentType: "application/json", a: v1beta1.AdmissionReview{TypeMeta: fixtureType, Request: &v1beta1.AdmissionRequest{UID: "1", Resource: fixtureResource, Object: fixtureObject}}},
		{name: "invalid content-type", responseCode: 415, responseBody: "invalid content-type", contentType: "application/text"},
		{name: "missing admission review", responseCode: 400, responseBody: "no admission review in request", contentType: "application/json"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			bb := new(bytes.Buffer)
			json.NewEncoder(bb).Encode(tc.a)
			req, err := http.NewRequest("POST", "localhost:8080/mutate", bb)
			if err != nil {
				t.Fatalf("could not create request %v", err)
			}
			req.Header.Set("Content-Type", tc.contentType)
			rec := httptest.NewRecorder()

			serve(rec, req)
			res := rec.Result()

			if res.StatusCode != tc.responseCode {
				t.Errorf("expected http response code %d but got %d", tc.responseCode, res.StatusCode)
			}

			b, err := ioutil.ReadAll(res.Body)
			body := strings.TrimSpace(string(b))

			if body != tc.responseBody {
				t.Fatalf("expected response body to be %s but got %s", tc.responseBody, body)
			}
		})
	}
}
