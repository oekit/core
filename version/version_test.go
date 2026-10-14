package version

import (
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/test/assert"
)

func TestMetadata(t *testing.T) {
	Version = "v0.8.0"
	BuildCommit = "abc1234"
	BuildBranch = "main"
	BuildTime = "2026-01-01"

	m := Metadata()
	assert.DeepEqual(t, "v0.8.0", m["version"])
	assert.DeepEqual(t, "abc1234", m["build_commit"])
	assert.DeepEqual(t, "main", m["build_branch"])
	assert.DeepEqual(t, "2026-01-01", m["build_time"])
}

func TestStartTime(t *testing.T) {
	if StartTime.IsZero() {
		t.Error("expected non-zero StartTime")
	}
	if time.Since(StartTime) > time.Minute {
		t.Error("StartTime should be recent")
	}
}
