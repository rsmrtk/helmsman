// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEC2(t *testing.T) {
	svc, ok := findService("aws", "ec2")
	require.True(t, ok)
	rows, err := parseCloud(svc, []byte(`{"Reservations":[
	 {"Instances":[
	  {"InstanceId":"i-2","State":{"Name":"stopped"},"InstanceType":"t3.small","PrivateIpAddress":"10.0.0.2","Tags":[{"Key":"Name","Value":"worker"}]},
	  {"InstanceId":"i-1","State":{"Name":"running"},"InstanceType":"m5.large","PrivateIpAddress":"10.0.0.1","PublicIpAddress":"3.3.3.3","Tags":[{"Key":"Name","Value":"api"}]}]}]}`))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"api", "i-1", "running", "m5.large", "10.0.0.1", "3.3.3.3"}, rows[0], "sorted by name")
	assert.Equal(t, "worker", rows[1][0])
	assert.Equal(t, "", rows[1][5], "a missing field is empty")
	assert.Contains(t, cloudText(svc, rows), "2 ec2")
}

func TestParseGKEAndVM(t *testing.T) {
	svc, _ := findService("gcp", "gke")
	rows, err := parseCloud(svc, []byte(`[{"name":"prod","status":"RUNNING","location":"europe-west3","currentMasterVersion":"1.31"}]`))
	require.NoError(t, err)
	assert.Equal(t, []string{"prod", "RUNNING", "europe-west3", "1.31"}, rows[0])

	vm, _ := findService("gcp", "vm")
	rows, err = parseCloud(vm, []byte(`[{"name":"a","status":"TERMINATED","machineType":"https://x/zones/z/machineTypes/e2-small","zone":"https://x/zones/europe-west3-a","networkInterfaces":[{"networkIP":"10.1.1.1"}]}]`))
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "TERMINATED", "e2-small", "europe-west3-a", "10.1.1.1"}, rows[0], "|last keeps the part after the final slash")
}

func TestParseEKSNames(t *testing.T) {
	svc, _ := findService("aws", "eks")
	rows, err := parseCloud(svc, []byte(`{"clusters":["b","a"]}`))
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"a"}, {"b"}}, rows)
}

func TestParseCloudBadJSON(t *testing.T) {
	svc, _ := findService("azure", "vm")
	_, err := parseCloud(svc, []byte("not json"))
	assert.Error(t, err)
}

func TestCloudStateKey(t *testing.T) {
	for in, want := range map[string]string{
		"running": "running", "RUNNING": "running", "VM running": "running", "stopped": "completed",
		"TERMINATED": "completed", "pending": "pending", "failed": "crashloopbackoff", "weird": "",
	} {
		assert.Equal(t, want, cloudStateKey(in), in)
	}
}

func TestProviderOf(t *testing.T) {
	for in, want := range map[string]string{"aws": "aws", "GCloud": "gcp", "az": "azure", "azure": "azure"} {
		p, ok := providerOf(in)
		assert.True(t, ok)
		assert.Equal(t, want, p)
	}
	_, ok := providerOf("oracle")
	assert.False(t, ok)
}

func TestImportPlan(t *testing.T) {
	cli, args, err := importPlan("aws", "prod", []string{"prod"}, "eu-central-1", "")
	require.NoError(t, err)
	assert.Equal(t, "aws", cli)
	assert.Equal(t, []string{"eks", "update-kubeconfig", "--name", "prod", "--region", "eu-central-1"}, args)

	_, _, err = importPlan("aws", "prod", []string{"prod"}, "", "")
	assert.Error(t, err, "no region configured")

	cli, args, err = importPlan("gcp", "prod", []string{"prod", "RUNNING", "europe-west3"}, "", "my-proj")
	require.NoError(t, err)
	assert.Equal(t, "gcloud", cli)
	assert.Contains(t, args, "--location")
	assert.Contains(t, args, "my-proj")

	cli, args, err = importPlan("azure", "prod", []string{"prod", "Succeeded", "1.31", "rg-1"}, "", "")
	require.NoError(t, err)
	assert.Equal(t, "az", cli)
	assert.Contains(t, args, "rg-1")
}

func TestCLIError(t *testing.T) {
	assert.Equal(t, "ERROR: (gcloud) API disabled", cliError("\nERROR: (gcloud) API disabled\n  reason: SERVICE_DISABLED\n", assert.AnError))
	assert.Equal(t, "plain failure", cliError("starting\nplain failure\n", assert.AnError))
	assert.Equal(t, assert.AnError.Error(), cliError("", assert.AnError))
}
