package cctv

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Camera struct {
	Name          string `json:"name"`
	RTSPURL       string `json:"rtsp_url"`
	RetentionDays int    `json:"retention_days"`
	RtspTransport string `json:"rtsp_transport"`
	Color         string `json:"color"`
}

type Config struct {
	ServeAddress  string   `json:"serve_address"`
	RetentionDays int      `json:"retention_days"`
	RtspTransport string   `json:"rtsp_transport"`
	Cameras       []Camera `json:"cameras"`
	StorageDir    string   `json:"storage_dir"`
	FfmpegBin     string   `json:"ffmpeg_bin"`
}

func (c *Config) Read(path string) error {
	// Defaults
	c.ServeAddress = ":8181"
	c.StorageDir = filepath.Join(Cwd, "storage")
	c.RetentionDays = 2
	c.RtspTransport = "tcp"

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c.Write(path)
		}
		return err
	}
	return json.Unmarshal(data, c)
}

func (c *Config) Write(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
