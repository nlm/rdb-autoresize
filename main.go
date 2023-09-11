package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/docker/go-units"
	"github.com/scaleway/scaleway-sdk-go/api/rdb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

var (
	flagTriggerPct      = flag.String("trigger-percentage", GetenvDefault("SCW_RDB_TRIGGER_PERCENTAGE", "90"), "disk resize trigger percentage")
	flagVolumeSizeLimit = flag.String("volume-size-limit", GetenvDefault("SCW_RDB_VOLUME_SIZE_LIMIT", "0GB"), "target volume size limit")
	flagLogJson         = flag.Bool("log-json", false, "use json format for logging")
	flagDebug           = flag.Bool("debug", false, "enable debug logging")
)

var (
	queryTimeout      = 1 * time.Minute
	diskSizeIncrement = uint64(5 * units.GB)
	loopInterval      = 5 * time.Minute
	appVersion        = "dev"
	userAgent         = "RDBAutoResize/" + appVersion
)

func GetenvDefault(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func setupLogging() {
	logLevel := slog.LevelInfo
	if *flagDebug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
	if *flagLogJson {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
	}
}

func parseOptions() (float64, int64, error) {
	// trigger percentage
	triggerPercent, err := strconv.ParseFloat(*flagTriggerPct, 64)
	if err != nil {
		return 0, 0, fmt.Errorf(
			"invalid trigger percentage '%s': %w",
			*flagTriggerPct,
			err,
		)
	}
	if triggerPercent >= 100 || triggerPercent < 80 {
		return 0, 0, fmt.Errorf("trigger percent must be between 80 and 100")
	}

	// volume size limit
	volumeSizeLimit, err := units.FromHumanSize(*flagVolumeSizeLimit)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid volume size limit: %w", err)
	}
	if volumeSizeLimit == 0 {
		return 0, 0, fmt.Errorf("limit is ZERO, no resize can happen")
	}

	return triggerPercent, volumeSizeLimit, nil
}

func makeAutoResizer() (*AutoResizer, error) {
	var options = []scw.ClientOption{
		scw.WithAuth(os.Getenv("SCW_ACCESS_KEY"), os.Getenv("SCW_SECRET_KEY")),
		scw.WithUserAgent(userAgent),
	}
	if *flagDebug {
		options = append(options, scw.WithHTTPClient(&http.Client{
			Transport: &loggingTransport{},
		}))
	}
	client, err := scw.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("error creating api client: %w", err)
	}
	return NewAutoResizer(client, os.Getenv("SCW_RDB_REGION"), os.Getenv("SCW_RDB_INSTANCE_ID")), nil
}

func main() {
	flag.Parse()
	setupLogging()

	// Parse options
	triggerPercent, volumeSizeLimit, err := parseOptions()
	if err != nil {
		slog.Error("error parsing options", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info(
		"rdb autoresizer started",
		slog.String("volume_size_limit", units.HumanSize(float64(volumeSizeLimit))),
		slog.Float64("trigger_percentage", triggerPercent),
		slog.String("version", appVersion),
	)

	// Creating API client and Helper
	rdbAR, err := makeAutoResizer()
	if err != nil {
		slog.Error("error creating api client", slog.Any("error", err))
		os.Exit(1)
	}

	// Check that instance exists, is compatible and that queries are working
	err = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		instance, err := rdbAR.GetInstance(ctx)
		if err != nil {
			return err
		}
		slog.Info(
			"rdb instance found",
			slog.Group("instance",
				slog.String("id", instance.ID),
				slog.String("name", instance.Name),
				slog.String("region", instance.Region.String()),
				slog.Group("volume",
					slog.String("type", instance.Volume.Type.String()),
					slog.String("size", units.HumanSize(float64(instance.Volume.Size))),
				),
			),
		)
		if instance.Volume.Type != rdb.VolumeTypeBssd {
			return fmt.Errorf("unsupported volume type: %s", instance.Volume.Type)
		}
		if int64(instance.Volume.Size) >= volumeSizeLimit {
			return fmt.Errorf("current volume size is larger than the defined limit")
		}
		return nil
	}()
	if err != nil {
		slog.Error("error during instance pre-checks", slog.Any("error", err))
		os.Exit(1)
	}

	// Control Loop
	slog.Debug("entering control loop", slog.Duration("interval", loopInterval))
	t := time.NewTicker(loopInterval)
	for ; ; <-t.C {
		// Check current usage
		v, err := func() (float64, error) {
			ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
			defer cancel()
			return rdbAR.GetDiskUsagePercent(ctx)
		}()
		if err != nil {
			slog.Error("error getting current disk usage", slog.Any("error", err))
			continue
		}
		slog.Info("current disk usage", slog.Float64("percent_used", v))

		// Take action
		if v > triggerPercent {
			slog.Warn(
				"disk space is over max usage target",
				slog.Float64("percent_target", triggerPercent),
				slog.Float64("percent_used", v),
			)

			// Check instance information
			instance, err := func() (*rdb.Instance, error) {
				ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
				defer cancel()
				return rdbAR.GetInstance(ctx)
			}()
			if err != nil {
				slog.Error("error getting instance details", slog.Any("error", err))
				continue
			}
			slog.Debug(
				"current volume size",
				slog.String("size", units.HumanSize(float64(instance.Volume.Size))),
			)
			if instance.Volume.Type != rdb.VolumeTypeBssd {
				slog.Error(
					"volume type is non-resizeable",
					slog.String("volume_type", instance.Volume.Type.String()),
				)
				os.Exit(1)
			}

			// Check size limit
			targetSize := uint64(instance.Volume.Size) + diskSizeIncrement
			if targetSize > uint64(volumeSizeLimit) {
				slog.Error(
					"new volume size is over limit",
					slog.String("target_size", units.HumanSize(float64(targetSize))),
					slog.String("limit_size", units.HumanSize(float64(volumeSizeLimit))),
				)
				os.Exit(1)
			}

			// Do the resize
			err = func() error {
				ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
				defer cancel()
				slog.Warn(
					"triggering resize",
					slog.String("current_size", units.HumanSize(float64(instance.Volume.Size))),
					slog.String("target_size", units.HumanSize(float64(targetSize))),
				)
				_, err := rdbAR.ResizeVolume(ctx, targetSize)
				return err

			}()
			if err != nil {
				slog.Error(
					"unable to resize instance",
					slog.Any("error", err),
				)
				continue
			}
		}
	}
}
