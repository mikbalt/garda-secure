package repository

// Page selects a window of a list (LIMIT / OFFSET). A zero Limit means no
// limit; exports use it after checking the total against their maximum.
type Page struct {
	Limit  int
	Offset int
}

// limit returns the LIMIT argument (nil = LIMIT NULL = no limit).
func (p Page) limit() any {
	if p.Limit <= 0 {
		return nil
	}
	return p.Limit
}
