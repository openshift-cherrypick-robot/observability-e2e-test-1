package utils

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"k8s.io/klog"
)

func ContainManagedClusterMetric(opt TestOptions, offset string) (error, bool) {
	grafanaConsoleURL := GetGrafanaURL(opt)
	path := "/api/datasources/proxy/1/api/v1/"
	queryParams := "query?query=%3Anode_memory_MemAvailable_bytes%3Asum%20offset%20" + offset
	req, err := http.NewRequest(
		"GET",
		grafanaConsoleURL+path+queryParams,
		nil)
	if err != nil {
		return err, false
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	token, err := FetchBearerToken(opt)
	if err != nil {
		return err, false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Host = opt.HubCluster.GrafanaHost

	resp, err := client.Do(req)
	if err != nil {
		return err, false
	}

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("resp.StatusCode: %v\n", resp.StatusCode)
		return fmt.Errorf("Failed to access managed cluster metrics via grafana console"), false
	}

	metricResult, err := ioutil.ReadAll(resp.Body)
	klog.V(1).Infof("metricResult: %s\n", metricResult)
	if err != nil {
		return err, false
	}

	if !strings.Contains(string(metricResult), `status":"success"`) {
		return fmt.Errorf("Failed to find valid status from response"), false
	}

	if !strings.Contains(string(metricResult), `"__name__":":node_memory_MemAvailable_bytes:sum"`) {
		return fmt.Errorf("Failed to find metric name from response"), false
	}

	return nil, true
}
