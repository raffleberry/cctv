package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Camera struct {
	Name          string `json:"name"`
	RTSPURL       string `json:"rtsp_url"`
	RetentionDays int    `json:"retention_days"`
	Color         string `json:"color"`
}

type Config struct {
	ServeAddress  string   `json:"serve_address"`
	RetentionDays int      `json:"retention_days"`
	Cameras       []Camera `json:"cameras"`
	StorageDir    string   `json:"storage_dir"`
	FfmpegBin     string   `json:"ffmpeg_bin"`
}

func (c *Config) Read(path string) error {
	defer func() {
		c.Write(path)
	}()

	c.ServeAddress = ":8181"
	c.StorageDir = filepath.Join(execDir, "storage")
	c.RetentionDays = 2

	data, err := os.ReadFile(path)
	if err != nil {
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
