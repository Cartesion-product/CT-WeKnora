// video-smoke checks the media probe and configured Ollama ASR against a real
// local MP4 before enabling video ingestion. It does not write to WeKnora.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/Tencent/WeKnora/internal/media"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/types"
)

func main() {
	model := flag.String("model", "frozenlab/qwen3-asr:1.7b", "installed Ollama ASR model")
	startWindow := flag.Int("start-window", 0, "zero-based 30-second window to resume from")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: video-smoke [-model name] [-start-window n] file.mp4")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	probeCtx, cancelProbe := context.WithTimeout(ctx, 30*time.Second)
	metadata, err := media.ProbeFile(probeCtx, flag.Arg(0), media.ProbeLimits{
		MaxBytes: 2 << 30, MaxDurationMS: int64(4 * time.Hour / time.Millisecond), MaxPixels: 3840 * 2160,
	})
	cancelProbe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(struct {
		Type string `json:"type"`
		media.VideoMetadata
	}{Type: "probe", VideoMetadata: metadata}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !metadata.HasAudio {
		return
	}
	modelInstance, err := asr.NewASR(&asr.Config{Source: types.ModelSourceLocal, ModelName: *model})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	transcriber, ok := modelInstance.(asr.WindowFileTranscriber)
	if !ok {
		fmt.Fprintln(os.Stderr, "configured ASR does not support streaming file windows")
		os.Exit(1)
	}
	if err := transcriber.TranscribeFileWindows(ctx, flag.Arg(0), *startWindow, func(window asr.Window) error {
		return encoder.Encode(struct {
			Type string `json:"type"`
			asr.Window
		}{Type: "asr_window", Window: window})
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
