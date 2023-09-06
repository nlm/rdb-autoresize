package main

import (
	"context"
	"fmt"

	rdb "github.com/scaleway/scaleway-sdk-go/api/rdb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

func NewAutoResizer(client *scw.Client, region, instance string) *AutoResizer {
	return &AutoResizer{
		rdbApi:     rdb.NewAPI(client),
		region:     scw.Region(region),
		instanceID: instance,
	}
}

type AutoResizer struct {
	rdbApi     *rdb.API
	region     scw.Region
	instanceID string
}

func (as AutoResizer) GetInstance(ctx context.Context) (*rdb.Instance, error) {
	return as.rdbApi.GetInstance(&rdb.GetInstanceRequest{
		Region:     as.region,
		InstanceID: as.instanceID,
	}, scw.WithContext(ctx))
}

func (as AutoResizer) ResizeVolume(ctx context.Context, newSize uint64) (*rdb.Instance, error) {
	instance, err := as.GetInstance(ctx)
	if err != nil {
		return nil, err
	}
	if instance.Status != rdb.InstanceStatusReady && instance.Status != rdb.InstanceStatusDiskFull {
		return nil, fmt.Errorf("instance is not in a ready state: %s", instance.Status)
	}
	return as.rdbApi.UpgradeInstance(&rdb.UpgradeInstanceRequest{
		Region:     as.region,
		InstanceID: as.instanceID,
		VolumeSize: &newSize,
	}, scw.WithContext(ctx))
}

func (as AutoResizer) GetDiskUsagePercent(ctx context.Context) (float64, error) {
	var metricName = "disk_usage_percent"
	metrics, err := as.rdbApi.GetInstanceMetrics(&rdb.GetInstanceMetricsRequest{
		Region:     as.region,
		InstanceID: as.instanceID,
		MetricName: &metricName,
	}, scw.WithContext(ctx))
	if err != nil {
		return 0, err
	}
	if len(metrics.Timeseries) != 1 && len(metrics.Timeseries[0].Points) != 1 {
		return 0, fmt.Errorf("malformed output")
	}
	return float64(metrics.Timeseries[0].Points[0].Value), nil
}
