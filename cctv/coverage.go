package cctv

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SlotSec is the HLS segment length written by recordLoop. The frontend
// seeks within segments via hls.js, so coverage is tracked at this
// granularity.
const SlotSec = 10

// SlotsPerBlock is the number of SlotSec slots in a 6h block.
const SlotsPerBlock = 6 * 3600 / SlotSec // 2160

var validBlocks = []string{"00-06", "06-12", "12-18", "18-24"}

func isValidBlock(b string) bool {
	for _, v := range validBlocks {
		if v == b {
			return true
		}
	}
	return false
}

func blockStartHour(block string) (int, error) {
	if !isValidBlock(block) {
		return 0, fmt.Errorf("invalid block %q", block)
	}
	return strconv.Atoi(block[:2])
}

func blockDir(cam, date, block string) string {
	return filepath.Join(config.StorageDir, cam, date, block)
}

// playlistStart parses the recording start time from a playlist filename
// (<DateTimeFormat>.m3u8).
func playlistStart(name string) (time.Time, bool) {
	base := strings.TrimSuffix(name, ".m3u8")
	t, err := time.ParseInLocation(DateTimeFormat, base, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// parseM3u8 reads an HLS playlist and returns per-segment durations and the
// referenced .ts filenames in order.
func parseM3u8Segments(path string) (durations []float64, segFiles []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	var pending float64
	hasPending := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#EXTINF:") {
			rest := strings.TrimPrefix(line, "#EXTINF:")
			if i := strings.Index(rest, ","); i >= 0 {
				rest = rest[:i]
			}
			d, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			if err != nil {
				continue
			}
			pending, hasPending = d, true
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if hasPending {
			durations = append(durations, pending)
			segFiles = append(segFiles, line)
			hasPending = false
		} else if strings.HasSuffix(line, ".ts") {
			// Segment without EXTINF (e.g. live sliding window edge).
			durations = append(durations, SlotSec)
			segFiles = append(segFiles, line)
		}
	}
	return durations, segFiles, sc.Err()
}

func sumDur(d []float64) float64 {
	var s float64
	for _, v := range d {
		s += v
	}
	return s
}

// describePlaylist implements the legacy Segment view over a playlist file.
func describePlaylist(path string) (Segment, error) {
	name := filepath.Base(path)
	start, ok := playlistStart(name)
	if !ok {
		return Segment{}, fmt.Errorf("unparsable playlist name %q", name)
	}
	durations, segFiles, err := parseM3u8Segments(path)
	if err != nil {
		return Segment{}, err
	}
	total := sumDur(durations)
	if total <= 0 {
		// Fall back to counting sibling .ts files.
		dir := filepath.Dir(path)
		prefix := strings.TrimSuffix(name, ".m3u8") + "."
		entries, _ := os.ReadDir(dir)
		n := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".ts") {
				n++
			}
		}
		total = float64(n * SlotSec)
		_ = segFiles
	}
	return Segment{Start: start, End: start.Add(time.Duration(total * float64(time.Second))), TsFileCount: len(durations)}, nil
}

// listDates returns sorted YYYY-MM-DD dirs present for a camera.
func listDates(cam string) []string {
	entries, err := os.ReadDir(filepath.Join(config.StorageDir, cam))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := time.Parse("2006-01-02", e.Name()); err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// listPlaylists returns sorted .m3u8 filenames in a block dir.
func listPlaylists(cam, date, block string) []string {
	entries, err := os.ReadDir(blockDir(cam, date, block))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".m3u8") {
			continue
		}
		if _, ok := playlistStart(e.Name()); !ok {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// countTs returns the number of .ts files in a block dir.
func countTs(cam, date, block string) int {
	entries, err := os.ReadDir(blockDir(cam, date, block))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ts") {
			n++
		}
	}
	return n
}

// blockCoverage builds the slot bitmap for one camera block: which 10s slots
// have a .ts on disk. It resolves each playlist's segments against the block
// start so gaps between recordings stay grey in the UI.
func blockCoverage(cam, date, block string) (coverage []int, playlists []PlaylistInfo) {
	hour, err := blockStartHour(block)
	if err != nil {
		return nil, nil
	}
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return nil, nil
	}
	blockStart := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, time.Local)

	dir := blockDir(cam, date, block)
	seen := map[int]bool{}
	for _, name := range listPlaylists(cam, date, block) {
		full := filepath.Join(dir, name)
		start, ok := playlistStart(name)
		if !ok {
			continue
		}
		durations, segFiles, err := parseM3u8Segments(full)
		if err != nil || len(segFiles) == 0 {
			continue
		}
		url := "/videos/" + cam + "/" + date + "/" + block + "/" + name
		total := sumDur(durations)
		playlists = append(playlists, PlaylistInfo{
			Name: name, URL: url, Start: start.Format(time.RFC3339), DurationSec: total,
		})
		// Walk segments in time order; only mark slots whose .ts exists.
		off := 0.0
		for i, seg := range segFiles {
			segStart := start.Add(time.Duration(off * float64(time.Second)))
			slot := int(segStart.Sub(blockStart).Seconds()+0.5) / SlotSec
			d := SlotSec
			if i < len(durations) && durations[i] > 0 {
				d = int(durations[i] + 0.5)
				if d <= 0 {
					d = SlotSec
				}
			}
			if slot >= 0 && slot < SlotsPerBlock {
				segPath := seg
				if !filepath.IsAbs(segPath) {
					segPath = filepath.Join(dir, filepath.Base(seg))
				}
				if _, err := os.Stat(segPath); err == nil {
					for s := slot; s < slot+d/SlotSec && s < SlotsPerBlock; s++ {
						seen[s] = true
					}
				}
			}
			off += durations[i]
		}
	}
	for s := range seen {
		coverage = append(coverage, s)
	}
	sort.Ints(coverage)
	sort.Slice(playlists, func(i, j int) bool { return playlists[i].Start < playlists[j].Start })
	return coverage, playlists
}
