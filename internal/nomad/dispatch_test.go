package nomad_test

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestDispatchForm(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/job/report": `{"ID": "report", "ParameterizedJob": {"Payload": "required", "MetaRequired": ["day"], "MetaOptional": ["region", "zone"]}}`,
	})

	form, err := client.DispatchForm(context.Background(), "production", "report")
	r.NoError(err)
	r.Equal("production", (*asked)[0].namespace)

	r.Equal(nomad.DispatchForm{
		Required: []string{"day"},
		Optional: []string{"region", "zone"},
		Payload:  nomad.PayloadRequired,
	}, form)
}

func TestDispatchForm_APayloadNotSaidIsOptional(t *testing.T) {
	r := require.New(t)

	// The cluster leaves the default out of the job.
	client, _ := clusterServer(t, map[string]string{"/v1/job/report": `{"ID": "report", "ParameterizedJob": {}}`})

	form, err := client.DispatchForm(context.Background(), "production", "report")
	r.NoError(err)
	r.Equal(nomad.PayloadOptional, form.Payload)
}

func TestDispatchForm_NotParameterized(t *testing.T) {
	r := require.New(t)

	client, _ := clusterServer(t, map[string]string{"/v1/job/web": `{"ID": "web"}`})

	_, err := client.DispatchForm(context.Background(), "production", "web")
	r.ErrorContains(err, "web is not a parameterized job")
}

func TestDispatchJob(t *testing.T) {
	r := require.New(t)

	client, asked := clusterServer(t, map[string]string{
		"/v1/job/report/dispatch": `{"DispatchedJobID": "report/dispatch-1790611292-1fcb1371", "EvalID": "eval-1"}`,
	})

	id, err := client.DispatchJob(context.Background(), "production", "report", map[string]string{"day": "monday"}, []byte("hello"))
	r.NoError(err)
	r.Equal("report/dispatch-1790611292-1fcb1371", id)

	sent := (*asked)[0]
	r.Equal("production", sent.namespace)
	r.Equal(map[string]any{"day": "monday"}, sent.body["Meta"])
	r.Equal(base64.StdEncoding.EncodeToString([]byte("hello")), sent.body["Payload"])
}
