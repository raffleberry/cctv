package cctv

import (
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var Cwd string
var execDir string
var logger *slog.Logger
var DEV = false

var retryBackoff = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	60 * time.Minute,
}

const DateTimeFormat = "2006-01-02_15-04-05"

func init() {
	var err error

	execDir, err = os.Executable()
	if err != nil {
		panic(err)
	}

	logLevel := slog.LevelInfo

	if isGoRun() || os.Getenv("DEV") != "" {
		DEV = true
		logLevel = slog.LevelDebug
	}

	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: DEV, Level: logLevel}))

	Cwd, err = os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	logger.Debug("Working Directory: " + Cwd)
	if DEV {
		logLevel = slog.LevelDebug
	}

}

func isGoRun() bool {
	return strings.HasPrefix(execDir, filepath.Join(os.TempDir(), "go-build"))
}

func checkFfmpeg() string {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err == nil {
		logger.Debug("Ffmpeg found", "Path", ffmpegPath)
	}

	if config.FfmpegBin != "" {
		err = exec.Command(config.FfmpegBin, "-version").Run()
		if err == nil {
			logger.Debug("Ffmpeg specified in config file is valid")
			ffmpegPath = config.FfmpegBin
		}
	}

	return ffmpegPath
}

func getBlockStart(t time.Time) time.Time {
	year, mon, day := t.Date()
	hour := t.Hour()
	blockNum := hour / 6
	blockHour := blockNum * 6
	return time.Date(year, mon, day, blockHour, 0, 0, 0, t.Location())
}
