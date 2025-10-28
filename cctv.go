package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var config Config

func main() {
	configPath := flag.String("config", filepath.Join(execDir, "config.json"), "Path to config file, default: ./config.json")
	flag.Parse()

	err := config.Read(*configPath)
	if err != nil {
		logger.Error("Failed to read config", "Error", err)
		panic(err)
	}

	logger.Debug("Using ", "Config", config)

	if checkFfmpeg() != "" {
		panic("FFmpeg not found")
	}

	err = os.MkdirAll(config.StorageDir, 0755)
	if err != nil {
		logger.Error("Failed to create storage directory", "Error", err)
		panic(err)
	}

	for _, cam := range config.Cameras {
		go recordLoop(cam)
		// go thumbnailPoller(cam)
	}
	go storageCleaner()

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.Dir(filepath.Join(execDir, "ui"))))
	mux.Handle("GET /videos/", http.StripPrefix("/videos/", http.FileServer(http.Dir(config.StorageDir))))
	mux.HandleFunc("GET /api/cameras", getCameras)
	mux.HandleFunc("GET /api/camera/", apiCamera)

	log.Println("Server starting on http://" + config.ServeAddress)
	err = http.ListenAndServe(config.ServeAddress, mux)
	if err != nil {
		log.Fatal(err)
	}
}

func apiCamera(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/camera/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	name := parts[0]
	var cam Camera
	for _, c := range config.Cameras {
		if c.Name == name {
			cam = c
			break
		}
	}
	if cam.Name == "" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if len(parts) == 1 {
		// Get available dates and segments
		avail := getAvailable(name)
		json.NewEncoder(w).Encode(avail)
	} else if len(parts) == 4 && parts[3] == "existing_segments" {
		// Get existing segments for date/seg
		date := parts[1]
		seg := parts[2]
		segs := getExistingSegments(name, date, seg)
		json.NewEncoder(w).Encode(segs)
	} else {
		http.NotFound(w, r)
	}
}

func getCameras(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config.Cameras)
}

type AvailableDate struct {
	Date     string   `json:"date"`
	Segments []string `json:"segments"`
}

func getAvailable(name string) []AvailableDate {
	camDir := filepath.Join(config.StorageDir, name)
	dateEntries, err := os.ReadDir(camDir)
	if err != nil {
		return nil
	}
	var avail []AvailableDate
	for _, d := range dateEntries {
		if !d.IsDir() {
			continue
		}
		dateStr := d.Name()
		dateDir := filepath.Join(camDir, dateStr)
		segEntries, err := os.ReadDir(dateDir)
		if err != nil {
			continue
		}
		var segList []string
		for _, s := range segEntries {
			if !s.IsDir() {
				continue
			}
			m3u8 := filepath.Join(dateDir, s.Name(), "stream.m3u8")
			if _, err := os.Stat(m3u8); err == nil {
				segList = append(segList, s.Name())
			}
		}
		if len(segList) > 0 {
			sort.Strings(segList)
			avail = append(avail, AvailableDate{Date: dateStr, Segments: segList})
		}
	}
	// Sort dates descending
	sort.Slice(avail, func(i, j int) bool {
		return avail[i].Date > avail[j].Date
	})
	return avail
}

func getExistingSegments(id, date, seg string) []int {
	dir := filepath.Join(config.StorageDir, id, date, seg)
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var nums []int
	for _, f := range files {
		name := f.Name()
		if strings.HasSuffix(name, ".ts") {
			numStr := strings.TrimSuffix(name, ".ts")
			num, err := strconv.Atoi(numStr)
			if err == nil {
				nums = append(nums, num)
			}
		}
	}
	sort.Ints(nums)
	return nums
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
		playlistName := fmt.Sprintf("%s.m3u8", now.Format(DateTimeFormat))
		m3u8 := filepath.Join(dir, playlistName)
		segmentPattern := filepath.Join(dir, "%04d.mp4")
		args := []string{
			"-i", cam.RTSPURL,
			"-t", strconv.Itoa(int(remaining.Seconds())),
			"-c:v", "copy",
			"-an", // remove audio
			"-f", "hls",
			"-hls_time", "10", // each segment, 10 second
			"-hls_list_size", "0",
			"-hls_segment_filename", segmentPattern,
		}

		args = append(args, m3u8)
		logger.Info("Record Starting", "Camera", cam.Name, "Date", dateStr,
			"Segment", segStr, "PlaylistName", m3u8, "FfmpegArgs", args)

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
