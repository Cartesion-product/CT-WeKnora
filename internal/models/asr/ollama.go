package asr

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
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

	parts := make([]string, 0, (len(pcm)+ollamaASRChunkBytes-1)/ollamaASRChunkBytes)
	for start := 0; start < len(pcm); start += ollamaASRChunkBytes {
		chunk := start/ollamaASRChunkBytes + 1
		end := min(start+ollamaASRChunkBytes, len(pcm))
		wav := ollamaASRWAV(pcm[start:end])
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
			return nil, fmt.Errorf("Ollama ASR chunk %d: %w", chunk, err)
		}
		_, transcript, ok := strings.Cut(answer.String(), "<asr_text>")
		if !ok {
			return nil, fmt.Errorf("Ollama ASR chunk %d returned no <asr_text> marker", chunk)
		}
		if text := strings.TrimSpace(transcript); text != "" {
			parts = append(parts, text)
		}
	}
	return &TranscriptionResult{
		Text:     strings.Join(parts, " "),
		Duration: float64(len(pcm)) / (ollamaASRSampleRate * 2),
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
