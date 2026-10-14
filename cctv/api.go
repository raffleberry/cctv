package cctv

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var errUnknownCamera = errors.New("unknown camera")

func getMux() *http.ServeMux {
	mux := http.NewServeMux()
	// Embedded UI (panics at startup when index.html is missing).
	mux.Handle("GET /videos/", http.StripPrefix("/videos/", http.FileServer(http.Dir(config.StorageDir))))
	mux.HandleFunc("GET /live/", liveHandler)
	mux.HandleFunc("GET /api/cameras", getCameras)
	mux.HandleFunc("GET /api/dates", apiDates)
	mux.HandleFunc("GET /api/blocks", apiBlocks)
	mux.HandleFunc("GET /api/timeline", apiTimeline)
	mux.HandleFunc("POST /api/live/", apiLiveStart)
	mux.HandleFunc("GET /api/camera/", apiCamera)
	// SPA fallback (must be last).
	mux.Handle("GET /", spaHandler())
	return mux
}

func getCameras(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config.Cameras)
}

func queryCams(r *http.Request) []string {
	raw := strings.TrimSpace(r.URL.Query().Get("cams"))
	if raw == "" {
		var all []string
		for _, c := range config.Cameras {
			all = append(all, c.Name)
		}
		return all
	}
	var out []string
	for _, c := range strings.Split(raw, ",") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// GET /api/dates?cams=a,b -> {cam: ["YYYY-MM-DD", ...]}
func apiDates(w http.ResponseWriter, r *http.Request) {
	out := map[string][]string{}
	for _, cam := range queryCams(r) {
		if _, ok := findCamera(cam); !ok {
			continue
		}
		dates := listDates(cam)
		if dates == nil {
			dates = []string{}
		}
		out[cam] = dates
	}
	writeJSON(w, out)
}

// GET /api/blocks?date=YYYY-MM-DD&cams=a,b -> {cam: [{block,present,ts_count,playlists}]}
func apiBlocks(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		http.Error(w, "bad date, want YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	out := map[string][]BlockPresence{}
	for _, cam := range queryCams(r) {
		if _, ok := findCamera(cam); !ok {
			continue
		}
		blocks := make([]BlockPresence, 0, len(validBlocks))
		for _, b := range validBlocks {
			names := listPlaylists(cam, date, b)
			urls := make([]string, 0, len(names))
			for _, n := range names {
				urls = append(urls, "/videos/"+cam+"/"+date+"/"+b+"/"+n)
			}
			blocks = append(blocks, BlockPresence{
				Block: b, Present: len(names) > 0,
				TsCount: countTs(cam, date, b), Playlists: urls,
			})
		}
		out[cam] = blocks
	}
	writeJSON(w, out)
}

// GET /api/timeline?date=..&block=00-06&cam=.. -> slot bitmap + playlists.
func apiTimeline(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	date, blk, cam := q.Get("date"), q.Get("block"), q.Get("cam")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		http.Error(w, "bad date", http.StatusBadRequest)
		return
	}
	if !isValidBlock(blk) {
		http.Error(w, "bad block, want one of 00-06 06-12 12-18 18-24", http.StatusBadRequest)
		return
	}
	if _, ok := findCamera(cam); !ok {
		http.NotFound(w, r)
		return
	}
	coverage, playlists := blockCoverage(cam, date, blk)
	if coverage == nil {
		coverage = []int{}
	}
	if playlists == nil {
		playlists = []PlaylistInfo{}
	}
	hour, _ := blockStartHour(blk)
	day, _ := time.ParseInLocation("2006-01-02", date, time.Local)
	blockStart := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, time.Local)
	writeJSON(w, map[string]any{
		"camera": cam, "date": date, "block": blk,
		"block_start": blockStart.Format(time.RFC3339),
		"slot_sec":    SlotSec, "slots": SlotsPerBlock,
		"coverage": coverage, "playlists": playlists,
	})
}

// POST /api/live/:cam/start -> starts on-demand live ffmpeg.
func apiLiveStart(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/live/")
	name = strings.TrimSuffix(name, "/start")
	name = strings.Trim(name, "/")
	if name == "" {
		http.Error(w, "missing camera", http.StatusBadRequest)
		return
	}
	dir, err := ensureLive(name)
	if err != nil {
		if errors.Is(err, errUnknownCamera) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = dir
	writeJSON(w, map[string]any{
		"camera": name, "url": "/live/" + name + "/index.m3u8",
	})
}

// liveHandler lazily starts ffmpeg then serves /live/:cam/... files.
func liveHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/live/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	cam := parts[0]
	if _, ok := findCamera(cam); !ok {
		http.NotFound(w, r)
		return
	}
	dir, err := ensureLive(cam)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	touchLive(cam)
	if strings.HasSuffix(parts[1], ".m3u8") {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	}
	http.ServeFile(w, r, filepath.Join(dir, filepath.Base(parts[1])))
}

// --- legacy endpoints (kept for compatibility) ---

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
				if s.IsDir() || !strings.HasSuffix(s.Name(), ".m3u8") {
					continue
				}
				m3u8 := filepath.Join(blockPath, s.Name())
				segment, err := parseM3u8(m3u8)
				if err != nil {
					logger.Error("Failed to parse m3u8", "Error", err, "file", m3u8)
					continue
				}
				block.Segments = append(block.Segments, segment)
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
	// Coverage slots for the block (10s granularity).
	hour, err := blockStartHour(seg)
	if err != nil {
		return nil
	}
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return nil
	}
	blockStart := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, time.Local)
	slotSet := map[int]bool{}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".m3u8") {
			continue
		}
		start, ok := playlistStart(name)
		if !ok {
			continue
		}
		durations, _, err := parseM3u8Segments(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		off := 0.0
		for _, d := range durations {
			slot := int(start.Add(time.Duration(off*float64(time.Second))).Sub(blockStart).Seconds()+0.5) / SlotSec
			if slot >= 0 && slot < SlotsPerBlock {
				slotSet[slot] = true
			}
			off += d
		}
	}
	var nums []int
	for s := range slotSet {
		nums = append(nums, s)
	}
	sort.Ints(nums)
	return nums
}
