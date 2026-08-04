package main

import (
	"testing"

	"gioui.org/app"
	"gioui.org/unit"
)

func TestApplyUIScaleOverride(t *testing.T) {
	t.Setenv(uiScaleEnv, "1.25")
	e := app.FrameEvent{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1.1}}
	got := applyUIScaleOverride(e)
	if got.Metric.PxPerDp != 1.25 || got.Metric.PxPerSp != 1.375 {
		t.Fatalf("metric = %+v, want PxPerDp 1.25 and PxPerSp 1.375", got.Metric)
	}
}

func TestApplyUIScaleOverrideRejectsInvalidValues(t *testing.T) {
	e := app.FrameEvent{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	for _, scale := range []string{"oops", "0.74", "2.01"} {
		t.Run(scale, func(t *testing.T) {
			t.Setenv(uiScaleEnv, scale)
			if got := applyUIScaleOverride(e); got.Metric != e.Metric {
				t.Fatalf("metric = %+v, want unchanged %+v", got.Metric, e.Metric)
			}
		})
	}
}
