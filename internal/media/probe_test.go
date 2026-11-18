package media

import (
	"strings"
	"testing"
)

const sampleProbe = `{"streams":[{"codec_type":"video","codec_name":"h264","width":1920,"height":1080},{"codec_type":"audio","codec_name":"aac"}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"61.125"}}`

func TestParseProbe(t *testing.T) {
	meta, err := ParseProbe([]byte(sampleProbe), 1024, ProbeLimits{MaxBytes: 2048, MaxDurationMS: 120000, MaxPixels: 1920 * 1080})
	if err != nil {
		t.Fatal(err)
	}
	if meta.DurationMS != 61125 || meta.VideoCodec != "h264" || meta.AudioCodec != "aac" || !meta.HasAudio || meta.Width != 1920 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestParseProbeRejectsBadMedia(t *testing.T) {
	cases := []struct {
		name   string
		data   string
		limits ProbeLimits
	}{
		{"not mp4", strings.Replace(sampleProbe, "mov,mp4,m4a,3gp,3g2,mj2", "matroska,webm", 1), ProbeLimits{}},
		{"no video", `{"streams":[{"codec_type":"audio","codec_name":"aac"}],"format":{"format_name":"mp4","duration":"10"}}`, ProbeLimits{}},
		{"zero duration", strings.Replace(sampleProbe, "61.125", "0", 1), ProbeLimits{}},
		{"too long", sampleProbe, ProbeLimits{MaxDurationMS: 60000}},
		{"too large", sampleProbe, ProbeLimits{MaxPixels: 1000000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseProbe([]byte(tc.data), 1024, tc.limits); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}
