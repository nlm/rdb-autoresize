package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"strconv"
	"time"

	// cockpit "github.com/scaleway/scaleway-sdk-go/api/cockpit/v1beta1"

	"github.com/docker/go-units"
	"github.com/scaleway/scaleway-sdk-go/api/rdb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

var (
	flagTriggerPct = flag.String("trigger-percentage", GetenvDefault("SCW_RDB_TRIGGER_PERCENTAGE", "90"), "disk usage percent trigger")
	flagLimit      = flag.String("volume-size-limit", GetenvDefault("SCW_RDB_VOLUME_SIZE_LIMIT", "0GB"), "volume size limit")
	flagJson       = flag.Bool("log-json", false, "log json")
	flagDebug      = flag.Bool("debug", false, "enable debug logging")
)

var (
	queryTimeout      = 1 * time.Minute
	diskSizeIncrement = uint64(5 * units.GB)
	loopInterval      = 1 * time.Minute
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
	if *flagJson {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
	}
}

func parseOptions() (float64, int64) {
	triggerPercent, err := strconv.ParseFloat(*flagTriggerPct, 64)
	if err != nil {
		slog.Error(
			"invalid trigger percentage",
			slog.String("value", *flagTriggerPct),
			slog.Any("error", err),
		)
		os.Exit(1)
	}
	if triggerPercent >= 100 || triggerPercent < 80 {
		slog.Error(
			"trigger percent must be between 80 and 100",
		)
		os.Exit(1)
	}

	limitSize, err := units.FromHumanSize(*flagLimit)
	if err != nil {
		slog.Error("invalid size limit", slog.Any("error", err))
		os.Exit(1)
	}
	if limitSize == 0 {
		slog.Error("limit is ZERO, no resize can happen")
		os.Exit(1)
	}

	return triggerPercent, limitSize
}

func main() {
	flag.Parse()
	setupLogging()

	// Parse options
	triggerPercent, limitSize := parseOptions()

	// Creating API client and Helper
	client, err := scw.NewClient(scw.WithAuth(os.Getenv("SCW_ACCESS_KEY"), os.Getenv("SCW_SECRET_KEY")))
	if err != nil {
		slog.Error("error creating api client", slog.Any("error", err))
		os.Exit(1)
	}
	rdbAR := NewAutoSizer(client, os.Getenv("SCW_RDB_REGION"), os.Getenv("SCW_RDB_INSTANCE_ID"))

	// Check that instance exists and that queries are working
	err = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		_, err = rdbAR.GetInstance(ctx)
		return err
	}()
	if err != nil {
		log.Fatal(err)
	}
	slog.Info(
		"rdb autoresizer started",
		slog.String("limit", units.HumanSize(float64(limitSize))),
		slog.Float64("trigger_percentage", triggerPercent),
	)

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

			// Check size limit
			targetSize := uint64(instance.Volume.Size) + diskSizeIncrement
			if targetSize > uint64(limitSize) {
				slog.Error(
					"new volume size is over limit",
					slog.String("target_size", units.HumanSize(float64(targetSize))),
					slog.String("limit_size", units.HumanSize(float64(limitSize))),
				)
				continue
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