package asr

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/utils/ollama"
	ollamaapi "github.com/ollama/ollama/api"
)

const (
	ollamaASRSampleRate  = 16000
	ollamaASRChunkBytes  = 30 * ollamaASRSampleRate * 2
	ollamaASRMaxPCMBytes = 30 * 60 * ollamaASRSampleRate * 2
)

type ollamaASR struct {
	modelName string
	modelID   string
	chat      func(context.Context, *ollamaapi.ChatRequest, ollamaapi.ChatResponseFunc) error
	decode    func(context.Context, []byte) ([]byte, error)
}

func newOllamaASR(config *Config) (ASR, error) {
	if strings.TrimSpace(config.ModelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if !strings.HasPrefix(config.ModelName, "frozenlab/qwen3-asr:") {
		return nil, fmt.Errorf("unsupported Ollama ASR model %q", config.ModelName)
	}
	service, err := ollama.GetOllamaService()
	if err != nil {
		return nil, err
	}
	return &ollamaASR{
		modelName: config.ModelName,
		modelID:   config.ModelID,
		chat:      service.Chat,
		decode:    decodeOllamaASRAudio,
	}, nil
}

func (a *ollamaASR) GetModelName() string { return a.modelName }
func (a *ollamaASR) GetModelID() string   { return a.modelID }

func (a *ollamaASR) Transcribe(ctx context.Context, audio []byte, _ string) (*TranscriptionResult, error) {
	if len(audio) == 0 {
		return nil, fmt.Errorf("audio bytes are empty")
	}
	pcm, err := a.decode(ctx, audio)
	if err != nil {
		return nil, err
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("audio contains no samples")
	}

	return a.transcribePCM(ctx, bytes.NewReader(pcm), 0, nil)
}

// TranscribeFile reads ffmpeg's PCM output one 30-second window at a time.
func (a *ollamaASR) TranscribeFile(ctx context.Context, path string) (*TranscriptionResult, error) {
	return a.transcribeFile(ctx, path, 0, nil)
}

func (a *ollamaASR) TranscribeFileWindows(ctx context.Context, path string, startWindow int, visit func(Window) error) error {
	if visit == nil {
		return fmt.Errorf("window callback is required")
	}
	_, err := a.transcribeFile(ctx, path, startWindow, visit)
	return err
}

func (a *ollamaASR) transcribeFile(ctx context.Context, path string, startWindow int, visit func(Window) error) (*TranscriptionResult, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("audio path is empty")
	}
	if startWindow < 0 {
		return nil, fmt.Errorf("start window must be non-negative")
	}
	args := []string{"-hide_banner", "-loglevel", "error"}
	if startWindow > 0 {
		args = append(args, "-ss", strconv.Itoa(startWindow*30))
	}
	args = append(args, "-i", path, "-vn", "-ac", "1", "-ar", "16000", "-f", "s16le", "pipe:1")
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	result, transcribeErr := a.transcribePCM(ctx, stdout, startWindow, visit)
	if transcribeErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if transcribeErr != nil {
		return nil, transcribeErr
	}
	if waitErr != nil {
		return nil, fmt.Errorf("decode audio: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return result, nil
}

func (a *ollamaASR) transcribePCM(ctx context.Context, pcm io.Reader, startWindow int, visit func(Window) error) (*TranscriptionResult, error) {
	parts := make([]string, 0)
	segments := make([]Segment, 0)
	buf := make([]byte, ollamaASRChunkBytes)
	var totalBytes int64
	for chunk := startWindow; ; chunk++ {
		n, readErr := io.ReadFull(pcm, buf)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("read decoded audio: %w", readErr)
		}
		if n == 0 {
			break
		}
		if n%2 != 0 {
			return nil, fmt.Errorf("decoded PCM ended on an incomplete sample")
		}
		wav := ollamaASRWAV(buf[:n])
		stream := false
		req := &ollamaapi.ChatRequest{
			Model:  a.modelName,
			Stream: &stream,
			Messages: []ollamaapi.Message{{
				Role: "user", Content: "", Images: []ollamaapi.ImageData{wav},
			}},
		}
		var answer strings.Builder
		if err := a.chat(ctx, req, func(resp ollamaapi.ChatResponse) error {
			answer.WriteString(resp.Message.Content)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("Ollama ASR chunk %d: %w", chunk+1, err)
		}
		_, transcript, ok := strings.Cut(answer.String(), "<asr_text>")
		if !ok {
			return nil, fmt.Errorf("Ollama ASR chunk %d returned no <asr_text> marker", chunk+1)
		}
		text := strings.TrimSpace(transcript)
		window := Window{
			Index: chunk,
			Start: float64(startWindow*30) + float64(totalBytes)/(ollamaASRSampleRate*2),
			End:   float64(startWindow*30) + float64(totalBytes+int64(n))/(ollamaASRSampleRate*2),
			Text:  text,
		}
		if visit != nil {
			if err := visit(window); err != nil {
				return nil, fmt.Errorf("save Ollama ASR window %d: %w", chunk+1, err)
			}
		}
		if text != "" {
			parts = append(parts, text)
			segments = append(segments, Segment{
				Start: window.Start,
				End:   window.End,
				Text:  text,
			})
		}
		totalBytes += int64(n)
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	if totalBytes == 0 {
		return nil, fmt.Errorf("audio contains no samples")
	}
	return &TranscriptionResult{
		Text:     strings.Join(parts, " "),
		Segments: segments,
		Duration: float64(startWindow*30) + float64(totalBytes)/(ollamaASRSampleRate*2),
	}, nil
}

func decodeOllamaASRAudio(ctx context.Context, audio []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-i", "pipe:0", "-ac", "1", "-ar", "16000", "-f", "s16le", "pipe:1")
	cmd.Stdin = bytes.NewReader(audio)
	var pcm bytes.Buffer
	output := &limitedPCMWriter{dst: &pcm, limit: ollamaASRMaxPCMBytes}
	cmd.Stdout = output
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if output.exceeded {
			return nil, fmt.Errorf("Ollama ASR audio exceeds 30 minutes")
		}
		return nil, fmt.Errorf("convert audio to 16 kHz mono PCM: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return pcm.Bytes(), nil
}

type limitedPCMWriter struct {
	dst      io.Writer
	limit    int
	count    int
	exceeded bool
}

func (w *limitedPCMWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.count {
		w.exceeded = true
		return 0, fmt.Errorf("decoded audio exceeds limit")
	}
	n, err := w.dst.Write(p)
	w.count += n
	return n, err
}

func ollamaASRWAV(pcm []byte) []byte {
	wav := make([]byte, 44+len(pcm))
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(36+len(pcm)))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], ollamaASRSampleRate)
	binary.LittleEndian.PutUint32(wav[28:], ollamaASRSampleRate*2)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav
}
