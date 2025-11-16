package cctv

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func getMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.Dir(filepath.Join(Cwd, "ui"))))
	mux.Handle("GET /videos/", http.StripPrefix("/videos/", http.FileServer(http.Dir(config.StorageDir))))
	mux.HandleFunc("GET /api/cameras", getCameras)
	mux.HandleFunc("GET /api/camera/", apiCamera)
	return mux
}

func getCameras(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config.Cameras)
}

func getCamData(name string) ([]CamData, error) {
	camDir := filepath.Join(config.StorageDir, name)
	dateEntries, err := os.ReadDir(camDir)
	var avail []CamData = make([]CamData, 0)
	if err != nil {
		return avail, err
	}
	for _, d := range dateEntries {
		if !d.IsDir() {
			continue
		}
		dateStr := d.Name()

		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		camData := CamData{Date: date}
		camData.Blocks = make([]Block, 0)

		dateDir := filepath.Join(camDir, dateStr)
		blockEntries, err := os.ReadDir(dateDir)
		if err != nil {
			continue
		}
		for _, be := range blockEntries {
			if !be.IsDir() {
				continue
			}
			blockName := be.Name()
			if len(blockName) != 5 || blockName[2] != '-' {
				continue
			}

			blockStart, err1 := strconv.Atoi(blockName[:2])
			blockEnd, err2 := strconv.Atoi(blockName[3:])

			if err1 != nil || err2 != nil {
				continue
			}

			block := Block{Start: blockStart, End: blockEnd}
			block.Segments = make([]Segment, 0)
			blockPath := filepath.Join(camDir, dateStr, blockName)
			segEntries, err := os.ReadDir(blockPath)

			if err != nil {
				logger.Error("Failed to parse block", "Error", err, "Name", camDir, "Date", dateStr, "Block", blockName)
				continue
			}

			for _, s := range segEntries {
				if !s.IsDir() {
					continue
				}
				if strings.HasSuffix(s.Name(), ".m3u8") {
					m3u8 := filepath.Join(blockPath, s.Name())
					if _, err := os.Stat(m3u8); err == nil {
						segment, err := parseM3u8(m3u8)
						if err != nil {
							logger.Error("Failed to parse m3u8", "Error", err, "file", m3u8)
							continue
						}
						block.Segments = append(block.Segments, segment)
					}

				}
			}

			camData.Blocks = append(camData.Blocks, block)

		}
		avail = append(avail, camData)
	}
	return avail, nil
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
		avail, err := getCamData(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
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
