package workhours

// Partial 是某一天与默认作息不同的地方。为空的项沿用默认作息。
type Partial struct {
	Start      *string `json:"start,omitempty"`
	LunchStart *string `json:"lunchStart,omitempty"`
	LunchEnd   *string `json:"lunchEnd,omitempty"`
	End        *string `json:"end,omitempty"`
}

// Apply 返回在 base 之上套用 p 之后的作息。
func (p Partial) Apply(base Hours) Hours {
	if p.Start != nil {
		base.Start = *p.Start
	}
	if p.LunchStart != nil {
		base.LunchStart = *p.LunchStart
	}
	if p.LunchEnd != nil {
		base.LunchEnd = *p.LunchEnd
	}
	if p.End != nil {
		base.End = *p.End
	}
	return base
}
