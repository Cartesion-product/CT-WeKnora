package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ProbeLimits bound resource use before ffmpeg decodes the source.
type ProbeLimits struct {
	MaxBytes      int64
	MaxDurationMS int64
	MaxPixels     int64
}

// VideoMetadata is derived from the media stream, never from the filename or
// the upload's Content-Type header.
type VideoMetadata struct {
	DurationMS int64  `json:"duration_ms"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec,omitempty"`
	HasAudio   bool   `json:"has_audio"`
	Size       int64  `json:"size"`
}

type probeDocument struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"`
	} `json:"streams"`
}

// ProbeFile inspects a local MP4 with ffprobe. The caller owns temporary-file
// cleanup and must pass a trusted local path created by the media worker.
func ProbeFile(ctx context.Context, path string, limits ProbeLimits) (VideoMetadata, error) {
	if !strings.EqualFold(filepath.Ext(path), ".mp4") {
		return VideoMetadata{}, fmt.Errorf("video must have an .mp4 extension")
	}
	info, err := os.Stat(path)
	if err != nil {
		return VideoMetadata{}, fmt.Errorf("stat video: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return VideoMetadata{}, fmt.Errorf("video is not a nonempty regular file")
	}
	if limits.MaxBytes > 0 && info.Size() > limits.MaxBytes {
		return VideoMetadata{}, fmt.Errorf("video exceeds the size limit")
	}
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries",
		"format=format_name,duration:stream=codec_type,codec_name,width,height,duration", "-of", "json", path)
	var stdout bytes.Buffer
	cmd.Stdout = &limitedWriter{Writer: &stdout, remaining: 1 << 20}
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{Writer: &stderr, remaining: 4096}
	if err := cmd.Run(); err != nil {
		return VideoMetadata{}, fmt.Errorf("ffprobe video: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseProbe(stdout.Bytes(), info.Size(), limits)
}

// ParseProbe validates ffprobe output independently of process execution.
func ParseProbe(data []byte, size int64, limits ProbeLimits) (VideoMetadata, error) {
	if size <= 0 || (limits.MaxBytes > 0 && size > limits.MaxBytes) {
		return VideoMetadata{}, fmt.Errorf("video has invalid or excessive size")
	}
	var doc probeDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return VideoMetadata{}, fmt.Errorf("invalid ffprobe JSON: %w", err)
	}
	if !strings.Contains(","+doc.Format.FormatName+",", ",mp4,") {
		return VideoMetadata{}, fmt.Errorf("media container is not MP4")
	}
	meta := VideoMetadata{Size: size}
	for _, stream := range doc.Streams {
		switch stream.CodecType {
		case "video":
			if meta.VideoCodec == "" && stream.Width > 0 && stream.Height > 0 {
				meta.VideoCodec = stream.CodecName
				meta.Width, meta.Height = stream.Width, stream.Height
				if doc.Format.Duration == "" {
					doc.Format.Duration = stream.Duration
				}
			}
		case "audio":
			meta.HasAudio = true
			if meta.AudioCodec == "" {
				meta.AudioCodec = stream.CodecName
			}
		}
	}
	if meta.VideoCodec == "" {
		return VideoMetadata{}, fmt.Errorf("MP4 has no decodable video stream")
	}
	if limits.MaxPixels > 0 && int64(meta.Width)*int64(meta.Height) > limits.MaxPixels {
		return VideoMetadata{}, fmt.Errorf("video resolution exceeds the limit")
	}
	seconds, err := strconv.ParseFloat(doc.Format.Duration, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > float64(math.MaxInt64)/1000 {
		return VideoMetadata{}, fmt.Errorf("video has no valid duration")
	}
	meta.DurationMS = int64(math.Ceil(seconds * 1000))
	if limits.MaxDurationMS > 0 && meta.DurationMS > limits.MaxDurationMS {
		return VideoMetadata{}, fmt.Errorf("video duration exceeds the limit")
	}
	return meta, nil
}

type limitedWriter struct {
	io.Writer
	remaining int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, fmt.Errorf("ffprobe output exceeds limit")
	}
	n, err := w.Writer.Write(p)
	w.remaining -= n
	return n, err
}
