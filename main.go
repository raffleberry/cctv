package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
)

func main() {
	rtspURL := "rtsp://user:pass@192.168.1.10:554/stream1"

	cmd := exec.Command("ffmpeg",
		"-i", rtspURL,
		"-c", "copy",
		"-an", // Remove audio
		"-f", "hls",
		"-hls_time", "10", // Each segment is 10 seconds
		"-hls_list_size", "17280", // Keep 17280 segments (2 days worth at 10s per segment)
		"-hls_flags", "+delete_segments+append_list", // Automatically delete older segments, append on restart
		"-hls_delete_threshold", "17400", // Safety buffer for deletion threshold
		"-strftime", "1", // expand hls filename
		"-hls_segment_filename", "hls/%Y-%m-%d_%H-%M-%S.ts", // Timestamped TS filenames
		"hls/stream.m3u8", // Save as HLS in the "hls" directory
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	go func() {
		// Run the FFmpeg command
		if err := cmd.Run(); err != nil {
			log.Fatalf("Error capturing RTSP stream: %v", err)
		}

		log.Println("Stream captured successfully.")
	}()

	http.Handle("/hls/", http.StripPrefix("/hls/", http.FileServer(http.Dir("./hls"))))

	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
