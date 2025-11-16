package cctv

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

var config Config

func Start() {
	configPath := flag.String("config", filepath.Join(Cwd, "config.json"), "Path to config file, default: ./config.json")
	flag.Parse()

	err := config.Read(*configPath)
	if err != nil {
		logger.Error("Failed to read config", "Error", err)
		panic(err)
	}

	logger.Debug("Using ", "Config", config)

	if checkFfmpeg() == "" {
		panic("FFmpeg not found")
	}

	err = os.MkdirAll(config.StorageDir, 0755)
	if err != nil {
		logger.Error("Failed to create storage directory", "Error", err)
		panic(err)
	}

	for _, cam := range config.Cameras {
		if cam.RTSPURL == "" {
			logger.Info("No Url for camera. Skipping...", "Name", cam.Name)
			continue
		}
		// go recordLoop(cam)
	}
	go storageCleaner()

	mux := getMux()

	log.Println("Server starting on http://" + config.ServeAddress)
	err = http.ListenAndServe(config.ServeAddress, mux)
	if err != nil {
		log.Fatal(err)
	}
}

func recordLoop(cam Camera) {
	retriesIdx := -1
	for {
		now := time.Now()
		blockStart := getBlockStart(now)
		blockEnd := blockStart.Add(6 * time.Hour)
		remaining := blockEnd.Sub(now)
		if remaining <= 0 {
			time.Sleep(time.Second)
			continue
		}
		dateStr := blockStart.Format("2006-01-02")
		segStr := fmt.Sprintf("%02d-%02d", blockStart.Hour(), blockStart.Hour()+6)
		dir := filepath.Join(config.StorageDir, cam.Name, dateStr, segStr)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			logger.Error("Failed to create storage sub-directory", "Error", err)
			panic(err)
		}
		playlistName := now.Format(DateTimeFormat)
		playlistM3u8 := filepath.Join(dir, playlistName+".m3u8")
		segmentFileName := filepath.Join(dir, playlistName+".%04d.ts")
		rtspTransport := config.RtspTransport
		if cam.RtspTransport != "" {
			rtspTransport = cam.RtspTransport
		}

		args := []string{
			"-hide_banner",
			"-progress", "pipe:1",
			"-i", cam.RTSPURL,
			"-rtsp_transport", rtspTransport,
			"-t", strconv.Itoa(int(remaining.Seconds())),
			"-c:v", "copy",
			"-an", // remove audio
			"-f", "hls",
			"-hls_time", "10", // each segment, 10 second
			"-hls_list_size", "0",
			"-hls_segment_filename", segmentFileName,
		}

		args = append(args, playlistM3u8)
		logger.Info("Record Starting", "Camera", cam.Name, "Date", dateStr,
			"Segment", segStr, "PlaylistName", playlistM3u8, "FfmpegArgs", args)

		cmd := exec.Command("ffmpeg", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			logger.Error("Failed to Record", "Camera", cam.Name, "Error", err)
			if retriesIdx != len(retryBackoff)-1 {
				retriesIdx++
			}
		}
		next := time.Now().Add(retryBackoff[retriesIdx]).Format(time.DateTime)
		logger.Info("Failed to Start Record - ", "NextTryAt", next)
		time.Sleep(retryBackoff[retriesIdx])
	}
}

// func thumbnailPoller(cam Camera) {
// 	for {
// 		time.Sleep(30 * time.Second)
// 		now := time.Now()
// 		blockStart := getBlockStart(now)
// 		dateStr := now.Format("2006-01-02")
// 		segStr := fmt.Sprintf("%02d-%02d", blockStart.Hour(), blockStart.Hour()+6)
// 		dir := filepath.Join(config.StorageDir, cam.Name, dateStr, segStr)
// 		thumbDir := filepath.Join(dir, "thumbnails")
// 		os.MkdirAll(thumbDir, 0755)
// 		files, err := os.ReadDir(dir)
// 		if err != nil {
// 			continue
// 		}
// 		for _, f := range files {
// 			name := f.Name()
// 			if !strings.HasSuffix(name, ".ts") {
// 				continue
// 			}
// 			numStr := strings.TrimSuffix(name, ".ts")
// 			thumbPath := filepath.Join(thumbDir, numStr+".jpg")
// 			if _, err := os.Stat(thumbPath); err == nil {
// 				continue
// 			}
// 			tsPath := filepath.Join(dir, name)
// 			args := []string{
// 				"-i", tsPath,
// 				"-ss", "0",
// 				"-vframes", "1",
// 				thumbPath,
// 			}
// 			cmd := exec.Command("ffmpeg", args...)
// 			err = cmd.Run()
// 			if err != nil {
// 				log.Printf("Thumbnail error for %s %s: %v", cam.Name, name, err)
// 			}
// 		}
// 	}
// }

func storageCleaner() {
	for {
		now := time.Now()
		for _, cam := range config.Cameras {
			rentiontionDays := config.RetentionDays
			if cam.RetentionDays != 0 {
				rentiontionDays = cam.RetentionDays
			}
			cutoff := now.AddDate(0, 0, -rentiontionDays).Format("2006-01-02")
			camDir := filepath.Join(config.StorageDir, cam.Name)
			dates, err := os.ReadDir(camDir)
			if err != nil {
				continue
			}
			for _, d := range dates {
				if !d.IsDir() {
					continue
				}
				dateStr := d.Name()
				if dateStr < cutoff {
					delDir := filepath.Join(camDir, dateStr)
					err := os.RemoveAll(delDir)
					if err != nil {
						log.Printf("Failed to delete %s: %v", delDir, err)
					} else {
						log.Printf("Deleted old data: %s", delDir)
					}
				}
			}
		}
		next := now.Add(24 * time.Hour).Sub(now) // tomorrow at 00:00
		time.Sleep(next)
	}
}
