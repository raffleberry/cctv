package cctv

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// On-demand live HLS: one ffmpeg per viewed camera writing 10s segments to a
// sliding window (not retained). Started when the user opens a live tile
// (POST /api/live/:cam/start, or lazily on /live/:cam/...), reaped after
// idleTimeout without viewers.

const liveIdleTimeout = 5 * time.Minute

type liveSession struct {
	cmd      *exec.Cmd
	lastView time.Time
}

var (
	liveMu       sync.Mutex
	liveSessions = map[string]*liveSession{}
)

func liveDir(cam string) string {
	return filepath.Join(config.StorageDir, "live", cam)
}

func findCamera(name string) (Camera, bool) {
	for _, c := range config.Cameras {
		if c.Name == name {
			return c, true
		}
	}
	return Camera{}, false
}

// ensureLive starts the live ffmpeg for cam if not running and returns the
// dir serving its index.m3u8.
func ensureLive(camName string) (string, error) {
	cam, ok := findCamera(camName)
	if !ok || cam.RTSPURL == "" {
		return "", errUnknownCamera
	}
	dir := liveDir(camName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	liveMu.Lock()
	defer liveMu.Unlock()
	if s, ok := liveSessions[camName]; ok && s.cmd != nil && s.cmd.Process != nil {
		s.lastView = time.Now()
		return dir, nil
	}

	rtspTransport := config.RtspTransport
	if cam.RtspTransport != "" {
		rtspTransport = cam.RtspTransport
	}
	args := []string{
		"-hide_banner",
		"-rtsp_transport", rtspTransport,
		"-i", cam.RTSPURL,
		"-c:v", "copy",
		"-an",
		"-f", "hls",
		"-hls_time", "10",
		"-hls_list_size", "6",
		"-hls_flags", "delete_segments+omit_endlist",
		"-hls_segment_filename", filepath.Join(dir, "%04d.ts"),
		filepath.Join(dir, "index.m3u8"),
	}
	cmd := exec.Command(ffmpegBin(), args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	liveSessions[camName] = &liveSession{cmd: cmd, lastView: time.Now()}
	go func(name string, c *exec.Cmd) {
		_ = c.Wait()
		liveMu.Lock()
		if s, ok := liveSessions[name]; ok && s.cmd == c {
			delete(liveSessions, name)
		}
		liveMu.Unlock()
		logger.Info("Live ffmpeg exited", "Camera", name)
	}(camName, cmd)
	logger.Info("Live started", "Camera", camName)
	return dir, nil
}

func touchLive(camName string) {
	liveMu.Lock()
	if s, ok := liveSessions[camName]; ok {
		s.lastView = time.Now()
	}
	liveMu.Unlock()
}

// reapLiveLoop stops live ffmpegs nobody watches anymore.
func reapLiveLoop() {
	for {
		time.Sleep(time.Minute)
		now := time.Now()
		liveMu.Lock()
		for name, s := range liveSessions {
			if now.Sub(s.lastView) > liveIdleTimeout && s.cmd != nil && s.cmd.Process != nil {
				_ = s.cmd.Process.Kill()
				delete(liveSessions, name)
				logger.Info("Live reaped (idle)", "Camera", name)
			}
		}
		liveMu.Unlock()
	}
}
