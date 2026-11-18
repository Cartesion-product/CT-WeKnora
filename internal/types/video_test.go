package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVideoEvidenceCoordinates(t *testing.T) {
	zero, thirty, frame := int64(0), int64(30000), int64(12500)
	cases := []struct {
		name string
		e    VideoEvidence
		ok   bool
	}{
		{"ASR at start", VideoEvidence{Modality: VideoModalityASR, Text: "hello", StartMS: &zero, EndMS: &thirty}, true},
		{"frame", VideoEvidence{Modality: VideoModalityOCR, Text: "slide", FrameMS: &frame}, true},
		{"frame at start", VideoEvidence{Modality: VideoModalityVLM, Text: "opening", FrameMS: &zero}, true},
		{"ASR without end", VideoEvidence{Modality: VideoModalityASR, Text: "hello", StartMS: &zero}, false},
		{"ASR outside source", VideoEvidence{Modality: VideoModalityASR, Text: "hello", StartMS: &zero, EndMS: &thirty}, false},
		{"frame falsely expanded", VideoEvidence{Modality: VideoModalityOCR, Text: "slide", FrameMS: &frame, StartMS: &zero}, false},
		{"empty text", VideoEvidence{Modality: VideoModalityVLM, FrameMS: &frame}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			duration := int64(60000)
			if tc.name == "ASR outside source" {
				duration = 29000
			}
			err := tc.e.Validate(duration)
			if (err == nil) != tc.ok {
				t.Fatalf("Validate() error = %v, want valid = %v", err, tc.ok)
			}
		})
	}
}

func TestSearchResultMediaIsOptional(t *testing.T) {
	plain, err := json.Marshal(SearchResult{ID: "chunk-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), `"media"`) {
		t.Fatalf("ordinary document unexpectedly contains media: %s", plain)
	}
	zero := int64(0)
	video, err := json.Marshal(SearchResult{ID: "chunk-2", Media: &MediaReference{
		Type: "video", SegmentID: "segment-1", EvidenceID: "evidence-1",
		Modality: VideoModalityASR, ProcessingRunID: "run-1", StartMS: &zero,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(video), `"start_ms":0`) {
		t.Fatalf("zero-based video citation lost its time: %s", video)
	}
}
