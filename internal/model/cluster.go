// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package model

import (
	"context"
	"errors"
	"time"

	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/dao"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/cache"
	mv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

const (
	clusterCacheSize   = 100
	clusterCacheExpiry = 1 * time.Minute
	clusterNodesKey    = "nodes"
)

type (
	// MetricsServer gather metrics information from pods and nodes.
	MetricsServer interface {
		MetricsService

		ClusterLoad(*v1.NodeList, *mv1beta1.NodeMetricsList, *client.ClusterMetrics) error
		NodesMetrics(*v1.NodeList, *mv1beta1.NodeMetricsList, client.NodesMetrics)
		PodsMetrics(*mv1beta1.PodMetricsList, client.PodsMetrics)
	}

	// MetricsService calls the metrics server for metrics info.
	MetricsService interface {
		HasMetrics() bool
		FetchNodesMetrics(ctx context.Context) (*mv1beta1.NodeMetricsList, error)
		FetchPodsMetrics(ctx context.Context, ns string) (*mv1beta1.PodMetricsList, error)
	}

	// Cluster represents a kubernetes resource.
	Cluster struct {
		factory dao.Factory
		mx      MetricsServer
		cache   *cache.LRUExpireCache
	}
)

// NewCluster returns a new cluster info resource.
func NewCluster(f dao.Factory) *Cluster {
	return &Cluster{
		factory: f,
		mx:      client.DialMetrics(f.Client()),
		cache:   cache.NewLRUExpireCache(clusterCacheSize),
	}
}

// Version returns the current K8s cluster version.
func (c *Cluster) Version() string {
	info, err := c.factory.Client().ServerVersion()
	if err != nil || info == nil {
		return client.NA
	}

	return info.GitVersion
}

// ContextName returns the context name.
func (c *Cluster) ContextName() string {
	n, err := c.factory.Client().Config().CurrentContextName()
	if err != nil {
		return client.NA
	}
	return n
}

// ClusterName returns the context name.
func (c *Cluster) ClusterName() string {
	n, err := c.factory.Client().Config().CurrentClusterName()
	if err != nil {
		return client.NA
	}
	return n
}

// UserName returns the user name.
func (c *Cluster) UserName() string {
	n, err := c.factory.Client().Config().CurrentUserName()
	if err != nil {
		return client.NA
	}
	return n
}

// Metrics gathers node level metrics and compute utilization percentages.
func (c *Cluster) Metrics(ctx context.Context, mx *client.ClusterMetrics) error {
	var (
		nn  *v1.NodeList
		err error
	)
	if v, ok := c.cache.Get(clusterNodesKey); ok {
		if nl, ok := v.(*v1.NodeList); ok {
			nn = nl
		}
	} else {
		if nn, err = dao.FetchNodes(ctx, c.factory, ""); err != nil {
			return err
		}
	}
	if nn == nil {
		return errors.New("unable to fetch nodes list")
	}
	if len(nn.Items) > 0 {
		c.cache.Add(clusterNodesKey, nn, clusterCacheExpiry)
	}
	var nmx *mv1beta1.NodeMetricsList
	if nmx, err = c.mx.FetchNodesMetrics(ctx); err != nil {
		// No metrics API (no metrics-server): fall back to what the pods request.
		if rerr := c.requestsLoad(ctx, nn, mx); rerr != nil {
			return err
		}
		return nil
	}

	return c.mx.ClusterLoad(nn, nmx, mx)
}

// requestsLoad reports how much of the cluster's allocatable CPU and memory the
// running pods have requested. It needs no metrics-server, so the header still
// says something useful on clusters without one.
func (c *Cluster) requestsLoad(ctx context.Context, nn *v1.NodeList, mx *client.ClusterMetrics) error {
	cs, err := c.factory.Client().Dial()
	if err != nil {
		return err
	}
	pods, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase!=Succeeded,status.phase!=Failed",
	})
	if err != nil {
		return err
	}

	var allocCPU, allocMEM, reqCPU, reqMEM int64
	for i := range nn.Items {
		a := nn.Items[i].Status.Allocatable
		allocCPU += a.Cpu().MilliValue()
		allocMEM += a.Memory().Value()
	}
	if allocCPU == 0 || allocMEM == 0 {
		return errors.New("nodes report no allocatable capacity")
	}
	for i := range pods.Items {
		for j := range pods.Items[i].Spec.Containers {
			r := pods.Items[i].Spec.Containers[j].Resources.Requests
			reqCPU += r.Cpu().MilliValue()
			reqMEM += r.Memory().Value()
		}
	}
	mx.PercCPU = int(reqCPU * 100 / allocCPU)
	mx.PercMEM = int(reqMEM * 100 / allocMEM)
	mx.FromRequests = true

	return nil
}
