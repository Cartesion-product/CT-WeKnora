package asr

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
)

// langfuseASR wraps an ASR implementation and reports each Transcribe call
// as a Langfuse generation observation. Audio bytes are not uploaded — we
// record file name, audio size and duration (as the vendor reports it, or
// from segment end times)
// so usage can be billed by second/minute, which is how most ASR providers
// price their services.
type langfuseASR struct {
	inner ASR
}

type langfuseFileASR struct {
	*langfuseASR
	file FileTranscriber
}

func (l *langfuseFileASR) TranscribeFile(ctx context.Context, path string) (*TranscriptionResult, error) {
	mgr := langfuse.GetManager()
	if !mgr.Enabled() {
		return l.file.TranscribeFile(ctx, path)
	}
	genCtx, gen := mgr.StartGeneration(ctx, langfuse.GenerationOptions{
		Name: "asr.transcribe_file", Model: l.inner.GetModelName(),
		Input:    map[string]interface{}{"file_name": filepath.Base(path)},
		Metadata: map[string]interface{}{"model_id": l.inner.GetModelID()},
	})
	result, err := l.file.TranscribeFile(genCtx, path)
	output := map[string]interface{}{}
	var duration float64
	if result != nil {
		duration = result.Duration
		output["duration_seconds"] = duration
		output["segment_count"] = len(result.Segments)
	}
	gen.Finish(output, asrDurationUsage(duration), err)
	return result, err
}

type langfuseWindowASR struct {
	*langfuseFileASR
	window WindowFileTranscriber
}

func (l *langfuseWindowASR) TranscribeFileWindows(ctx context.Context, path string, startWindow int, visit func(Window) error) error {
	if visit == nil {
		return fmt.Errorf("window callback is required")
	}
	mgr := langfuse.GetManager()
	if !mgr.Enabled() {
		return l.window.TranscribeFileWindows(ctx, path, startWindow, visit)
	}
	genCtx, gen := mgr.StartGeneration(ctx, langfuse.GenerationOptions{
		Name: "asr.transcribe_file_windows", Model: l.inner.GetModelName(),
		Input:    map[string]interface{}{"file_name": filepath.Base(path), "start_window": startWindow},
		Metadata: map[string]interface{}{"model_id": l.inner.GetModelID()},
	})
	count := 0
	var lastEnd float64
	err := l.window.TranscribeFileWindows(genCtx, path, startWindow, func(window Window) error {
		if err := visit(window); err != nil {
			return err
		}
		count++
		lastEnd = window.End
		return nil
	})
	processedSeconds := float64(0)
	if count > 0 {
		processedSeconds = lastEnd - float64(startWindow*30)
	}
	gen.Finish(map[string]interface{}{"window_count": count, "processed_seconds": processedSeconds},
		asrDurationUsage(processedSeconds), err)
	return err
}

func asrDurationUsage(seconds float64) *langfuse.TokenUsage {
	if seconds <= 0 {
		return nil
	}
	n := int(seconds + 0.5)
	return &langfuse.TokenUsage{Output: n, Total: n, Unit: "SECONDS"}
}

func (l *langfuseASR) GetModelName() string { return l.inner.GetModelName() }
func (l *langfuseASR) GetModelID() string   { return l.inner.GetModelID() }

func (l *langfuseASR) Transcribe(ctx context.Context, audioBytes []byte, fileName string) (*TranscriptionResult, error) {
	mgr := langfuse.GetManager()
	if !mgr.Enabled() {
		return l.inner.Transcribe(ctx, audioBytes, fileName)
	}

	genCtx, gen := mgr.StartGeneration(ctx, langfuse.GenerationOptions{
		Name:  "asr.transcribe",
		Model: l.inner.GetModelName(),
		Input: map[string]interface{}{
			"file_name":  fileName,
			"audio_size": len(audioBytes),
		},
		Metadata: map[string]interface{}{
			"model_id":   l.inner.GetModelID(),
			"audio_size": len(audioBytes),
		},
	})

	result, err := l.inner.Transcribe(genCtx, audioBytes, fileName)

	output := map[string]interface{}{}
	var duration float64
	if result != nil {
		output["text"] = result.Text
		output["segment_count"] = len(result.Segments)
		// The vendor's own figure when it gives one; otherwise the end of
		// the last segment, which only verbose replies carry.
		duration = result.Duration
		if n := len(result.Segments); duration == 0 && n > 0 {
			duration = result.Segments[n-1].End
		}
		if duration > 0 {
			output["duration_seconds"] = duration
		}
	}

	// ASR is billed per second; we emit the audio duration (when available)
	// in the "Output" side of usage so users can configure per-model pricing
	// in Langfuse (e.g. $0.006 per minute for Whisper-1).
	var usage *langfuse.TokenUsage
	if duration > 0 {
		seconds := int(duration + 0.5)
		usage = &langfuse.TokenUsage{
			Output: seconds,
			Total:  seconds,
			Unit:   "SECONDS",
		}
	}

	gen.Finish(output, usage, err)
	return result, err
}

// wrapASRLangfuse applies the Langfuse decorator when the manager is enabled.
func wrapASRLangfuse(a ASR, err error) (ASR, error) {
	if err != nil || a == nil {
		return a, err
	}
	if !langfuse.GetManager().Enabled() {
		return a, nil
	}
	base := &langfuseASR{inner: a}
	file, hasFile := a.(FileTranscriber)
	if !hasFile {
		return base, nil
	}
	fileWrapper := &langfuseFileASR{langfuseASR: base, file: file}
	if window, ok := a.(WindowFileTranscriber); ok {
		return &langfuseWindowASR{langfuseFileASR: fileWrapper, window: window}, nil
	}
	return fileWrapper, nil
}
