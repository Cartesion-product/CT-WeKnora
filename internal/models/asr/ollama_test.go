package asr

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	ollamaapi "github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"
)

func TestOllamaASRTranscribesChunks(t *testing.T) {
	pcm := bytes.Repeat([]byte{1, 2}, ollamaASRChunkBytes/2+1)
	calls := 0
	a := &ollamaASR{
		modelName: "frozenlab/qwen3-asr:1.7b",
		modelID:   "ct-weknora-asr",
		decode: func(_ context.Context, audio []byte) ([]byte, error) {
			require.Equal(t, []byte("input"), audio)
			return pcm, nil
		},
		chat: func(_ context.Context, req *ollamaapi.ChatRequest, fn ollamaapi.ChatResponseFunc) error {
			calls++
			require.Equal(t, "frozenlab/qwen3-asr:1.7b", req.Model)
			require.False(t, *req.Stream)
			require.Len(t, req.Messages, 1)
			require.Empty(t, req.Messages[0].Content)
			require.Len(t, req.Messages[0].Images, 1)
			wav := []byte(req.Messages[0].Images[0])
			require.Equal(t, "RIFF", string(wav[:4]))
			require.Equal(t, "WAVE", string(wav[8:12]))
			require.Equal(t, uint32(ollamaASRSampleRate), binary.LittleEndian.Uint32(wav[24:28]))
			require.Equal(t, uint32(len(wav)-44), binary.LittleEndian.Uint32(wav[40:44]))
			return fn(ollamaapi.ChatResponse{Message: ollamaapi.Message{
				Content: "language Chinese<asr_text>第" + string(rune('0'+calls)) + "段",
			}})
		},
	}

	result, err := a.Transcribe(context.Background(), []byte("input"), "voice.webm")
	require.NoError(t, err)
	require.Equal(t, "第1段 第2段", result.Text)
	require.InDelta(t, 30.0000625, result.Duration, 0.000001)
	require.Equal(t, []Segment{
		{Start: 0, End: 30, Text: "第1段"},
		{Start: 30, End: 30.0000625, Text: "第2段"},
	}, result.Segments)
	require.Equal(t, 2, calls)
	require.Equal(t, "ct-weknora-asr", a.GetModelID())
}

func TestOllamaASRSkipsSilentWindowButKeepsAbsoluteTime(t *testing.T) {
	calls := 0
	a := &ollamaASR{modelName: "frozenlab/qwen3-asr:1.7b",
		chat: func(_ context.Context, _ *ollamaapi.ChatRequest, fn ollamaapi.ChatResponseFunc) error {
			calls++
			text := ""
			if calls == 2 {
				text = "第二段"
			}
			return fn(ollamaapi.ChatResponse{Message: ollamaapi.Message{Content: "<asr_text>" + text}})
		},
	}
	pcm := bytes.NewReader(bytes.Repeat([]byte{0, 0}, ollamaASRChunkBytes))
	result, err := a.transcribePCM(context.Background(), pcm, 0, nil)
	require.NoError(t, err)
	require.Equal(t, "第二段", result.Text)
	require.Equal(t, []Segment{{Start: 30, End: 60, Text: "第二段"}}, result.Segments)
}

func TestOllamaASRWindowCallbackIncludesSilenceAndResumeOffset(t *testing.T) {
	a := &ollamaASR{modelName: "frozenlab/qwen3-asr:1.7b",
		chat: func(_ context.Context, _ *ollamaapi.ChatRequest, fn ollamaapi.ChatResponseFunc) error {
			return fn(ollamaapi.ChatResponse{Message: ollamaapi.Message{Content: "<asr_text>"}})
		},
	}
	var windows []Window
	result, err := a.transcribePCM(context.Background(), bytes.NewReader([]byte{0, 0}), 3, func(w Window) error {
		windows = append(windows, w)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []Window{{Index: 3, Start: 90, End: 90.0000625}}, windows)
	require.Empty(t, result.Segments)
	require.InDelta(t, 90.0000625, result.Duration, 0.000001)
}

func TestOllamaASRRejectsUnexpectedResponse(t *testing.T) {
	a := &ollamaASR{
		decode: func(context.Context, []byte) ([]byte, error) { return []byte{0, 0}, nil },
		chat: func(_ context.Context, _ *ollamaapi.ChatRequest, fn ollamaapi.ChatResponseFunc) error {
			return fn(ollamaapi.ChatResponse{Message: ollamaapi.Message{Content: "language English"}})
		},
	}
	_, err := a.Transcribe(context.Background(), []byte("input"), "voice.wav")
	require.ErrorContains(t, err, "<asr_text>")
}

func TestOllamaASRPropagatesCallFailure(t *testing.T) {
	a := &ollamaASR{
		decode: func(context.Context, []byte) ([]byte, error) { return []byte{0, 0}, nil },
		chat: func(context.Context, *ollamaapi.ChatRequest, ollamaapi.ChatResponseFunc) error {
			return errors.New("upstream failed")
		},
	}
	_, err := a.Transcribe(context.Background(), []byte("input"), "voice.wav")
	require.ErrorContains(t, err, "upstream failed")
}

func TestNewASRUsesOllamaForLocalModel(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://localhost:11434")
	a, err := NewASR(&Config{Source: types.ModelSourceLocal, ModelName: "frozenlab/qwen3-asr:1.7b"})
	require.NoError(t, err)
	require.Equal(t, "frozenlab/qwen3-asr:1.7b", a.GetModelName())
	_, err = NewASR(&Config{Source: types.ModelSourceLocal, ModelName: "other-asr:latest"})
	require.ErrorContains(t, err, "unsupported Ollama ASR model")
}

func TestLimitedPCMWriter(t *testing.T) {
	var buf bytes.Buffer
	w := &limitedPCMWriter{dst: &buf, limit: 4}
	_, err := w.Write([]byte("1234"))
	require.NoError(t, err)
	_, err = w.Write([]byte("5"))
	require.Error(t, err)
	require.True(t, w.exceeded)
	require.Equal(t, "1234", strings.TrimSpace(buf.String()))
}
