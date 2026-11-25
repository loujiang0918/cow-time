package workhours

import "testing"

func TestPartialApply(t *testing.T) {
	base := Default()

	if got := (Partial{}).Apply(base); got != base {
		t.Errorf("没有任何字段的覆盖项应该保持默认作息，得到 %+v", got)
	}

	late := "20:30"
	want := Hours{Start: "09:00", LunchStart: "12:00", LunchEnd: "13:00", End: "20:30"}
	if got := (Partial{End: &late}).Apply(base); got != want {
		t.Errorf("只覆盖下班时间得到 %+v，期望 %+v", got, want)
	}

	start, lunchStart, lunchEnd, end := "10:00", "13:00", "14:00", "19:00"
	all := Partial{Start: &start, LunchStart: &lunchStart, LunchEnd: &lunchEnd, End: &end}
	if got, want := all.Apply(base), (Hours{Start: start, LunchStart: lunchStart, LunchEnd: lunchEnd, End: end}); got != want {
		t.Errorf("四项都覆盖得到 %+v，期望 %+v", got, want)
	}

	if base != Default() {
		t.Error("套用覆盖项不应该改动默认作息本身")
	}
}
