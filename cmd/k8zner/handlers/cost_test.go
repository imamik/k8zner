package handlers

import (
	"testing"

	"github.com/imamik/k8zner/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDesiredResourcesFromConfig(t *testing.T) {
	t.Run("always includes kube API and ingress LBs", func(t *testing.T) {
		cfg := &config.Config{
			ClusterName: "test",
			Location:    "nbg1",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Name: "cp", ServerType: "cx22", Count: 1, Location: "nbg1"},
				},
			},
			Addons: config.AddonsConfig{
				Traefik: config.DefaultTraefik(true),
			},
		}

		resources := desiredResourcesFromConfig(cfg)

		lbCount := 0
		for _, r := range resources {
			if r.kind == "lb" {
				lbCount += r.count
			}
		}
		assert.Equal(t, 2, lbCount, "should have 2 LBs: kube API + Traefik ingress")
	})

	t.Run("only kube API LB when Traefik disabled", func(t *testing.T) {
		cfg := &config.Config{
			ClusterName: "test",
			Location:    "nbg1",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Name: "cp", ServerType: "cx22", Count: 1, Location: "nbg1"},
				},
			},
			Addons: config.AddonsConfig{
				Traefik: config.DefaultTraefik(false),
			},
		}

		resources := desiredResourcesFromConfig(cfg)

		lbCount := 0
		for _, r := range resources {
			if r.kind == "lb" {
				lbCount += r.count
			}
		}
		assert.Equal(t, 1, lbCount, "should have 1 LB: kube API only")
	})

	t.Run("uses custom ingress LB type", func(t *testing.T) {
		cfg := &config.Config{
			ClusterName: "test",
			Location:    "nbg1",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Name: "cp", ServerType: "cx22", Count: 1, Location: "nbg1"},
				},
			},
			Ingress: config.IngressConfig{
				LoadBalancerType: "lb21",
			},
			Addons: config.AddonsConfig{
				Traefik: config.DefaultTraefik(true),
			},
		}

		resources := desiredResourcesFromConfig(cfg)

		var lbTypes []string
		for _, r := range resources {
			if r.kind == "lb" {
				lbTypes = append(lbTypes, r.typeName)
			}
		}
		assert.Contains(t, lbTypes, "lb11", "kube API LB is always lb11")
		assert.Contains(t, lbTypes, "lb21", "ingress LB should use custom type")
	})

	t.Run("counts all server pools", func(t *testing.T) {
		cfg := &config.Config{
			ClusterName: "test",
			Location:    "nbg1",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Name: "cp", ServerType: "cx22", Count: 3, Location: "nbg1"},
				},
			},
			Workers: []config.WorkerNodePool{
				{Name: "w1", ServerType: "cx32", Count: 2, Location: "nbg1"},
				{Name: "w2", ServerType: "cx42", Count: 4, Location: "fsn1"},
			},
			Addons: config.AddonsConfig{
				Traefik: config.DefaultTraefik(true),
			},
		}

		resources := desiredResourcesFromConfig(cfg)

		serverCount := 0
		for _, r := range resources {
			if r.kind == "server" {
				serverCount += r.count
			}
		}
		assert.Equal(t, 9, serverCount, "3 CP + 2 w1 + 4 w2")
	})

	t.Run("skips zero-count pools", func(t *testing.T) {
		cfg := &config.Config{
			ClusterName: "test",
			Location:    "nbg1",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Name: "cp", ServerType: "cx22", Count: 0, Location: "nbg1"},
				},
			},
			Workers: []config.WorkerNodePool{
				{Name: "w1", ServerType: "cx32", Count: 0, Location: "nbg1"},
			},
			Addons: config.AddonsConfig{
				Traefik: config.DefaultTraefik(true),
			},
		}

		resources := desiredResourcesFromConfig(cfg)

		for _, r := range resources {
			assert.NotEqual(t, "server", r.kind, "zero-count pools should not generate server resources")
		}
	})

	t.Run("includes additional LB pools", func(t *testing.T) {
		cfg := &config.Config{
			ClusterName: "test",
			Location:    "nbg1",
			IngressLoadBalancerPools: []config.IngressLoadBalancerPool{
				{Name: "extra", Type: "lb31", Count: 2, Location: "fsn1"},
			},
			Addons: config.AddonsConfig{
				Traefik: config.DefaultTraefik(true),
			},
		}

		resources := desiredResourcesFromConfig(cfg)

		lbCount := 0
		for _, r := range resources {
			if r.kind == "lb" {
				lbCount += r.count
			}
		}
		assert.Equal(t, 4, lbCount, "1 kube API + 1 ingress + 2 extra pool")
	})

	t.Run("defaults pool location to cluster location", func(t *testing.T) {
		cfg := &config.Config{
			ClusterName: "test",
			Location:    "nbg1",
			ControlPlane: config.ControlPlaneConfig{
				NodePools: []config.ControlPlaneNodePool{
					{Name: "cp", ServerType: "cx22", Count: 1}, // no Location
				},
			},
			Workers: []config.WorkerNodePool{
				{Name: "w1", ServerType: "cx32", Count: 1}, // no Location
			},
			Addons: config.AddonsConfig{
				Traefik: config.DefaultTraefik(true),
			},
		}

		resources := desiredResourcesFromConfig(cfg)

		for _, r := range resources {
			assert.Equal(t, "nbg1", r.location, "resource %s should default to cluster location", r.typeName)
		}
	})
}

func TestS3MonthlyCost(t *testing.T) {
	t.Run("base cost for anything up to 1TB", func(t *testing.T) {
		assert.Equal(t, hetznerS3BaseMonthlyEUR, s3MonthlyCost(100))
		assert.Equal(t, hetznerS3BaseMonthlyEUR, s3MonthlyCost(1000))
	})

	t.Run("S3 cost scales beyond 1TB", func(t *testing.T) {
		cost := s3MonthlyCost(2000)
		expected := hetznerS3BaseMonthlyEUR + 1000*hetznerS3PerGBExtraEUR
		assert.InDelta(t, expected, cost, 0.001)
	})
}

func TestAddOrMerge(t *testing.T) {
	t.Run("merges same-name items", func(t *testing.T) {
		items := []costLineItem{
			{Name: "load-balancer:lb11", Count: 1, UnitNet: 5.0, MonthlyNet: 5.0},
		}
		addOrMerge(&items, costLineItem{Name: "load-balancer:lb11", Count: 1, UnitNet: 5.0, MonthlyNet: 5.0})

		require.Len(t, items, 1)
		assert.Equal(t, 2, items[0].Count)
		assert.InDelta(t, 10.0, items[0].MonthlyNet, 0.001)
	})

	t.Run("appends different-name items", func(t *testing.T) {
		items := []costLineItem{
			{Name: "load-balancer:lb11", Count: 1, UnitNet: 5.0, MonthlyNet: 5.0},
		}
		addOrMerge(&items, costLineItem{Name: "server:cx22", Count: 1, UnitNet: 10.0, MonthlyNet: 10.0})

		require.Len(t, items, 2)
	})
}

func TestSumCost(t *testing.T) {
	items := []costLineItem{
		{Name: "a", Count: 2, MonthlyNet: 10.0, MonthlyGross: 12.0},
		{Name: "b", Count: 1, MonthlyNet: 5.0, MonthlyGross: 6.0},
	}

	total := sumCost("total", items)
	assert.Equal(t, 3, total.Count)
	assert.InDelta(t, 15.0, total.MonthlyNet, 0.001)
	assert.InDelta(t, 18.0, total.MonthlyGross, 0.001)
}
