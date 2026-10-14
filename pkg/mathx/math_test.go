package mathx

import (
	"testing"
)

func TestRandIntn(t *testing.T) {
	for i := 0; i < 100; i++ {
		v := RandIntn(10)
		if v < 0 || v >= 10 {
			t.Errorf("RandIntn(10) = %v, want [0, 10)", v)
		}
	}
}

func TestDivisionN(t *testing.T) {
	tests := []struct {
		name     string
		dividend float64
		divisor  float64
		n        int
		want     float64
	}{
		{"normal", 10, 3, 2, 3.33},
		{"zero dividend", 0, 3, 2, 0},
		{"zero divisor", 10, 0, 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DivisionN(tt.dividend, tt.divisor, tt.n); got != tt.want {
				t.Errorf("DivisionN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRound(t *testing.T) {
	tests := []struct {
		name string
		x    float64
		n    int
		want float64
	}{
		{"round up", 3.456, 2, 3.46},
		{"round down", 3.454, 2, 3.45},
		{"integer", 3.0, 0, 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Round(tt.x, tt.n); got != tt.want {
				t.Errorf("Round() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFloor(t *testing.T) {
	if got := Floor(3.999, 2); got != 3.99 {
		t.Errorf("Floor() = %v, want 3.99", got)
	}
}

func TestToFixed(t *testing.T) {
	if got := ToFixed(3.456, 2); got != 3.46 {
		t.Errorf("ToFixed() = %v, want 3.46", got)
	}
}

func TestRangeRandFloat64(t *testing.T) {
	for i := 0; i < 100; i++ {
		v := RangeRandFloat64(1.0, 2.0)
		if v < 1.0 || v >= 2.0 {
			t.Errorf("RangeRandFloat64(1.0, 2.0) = %v, want [1.0, 2.0)", v)
		}
	}
}

func TestProbabilityElems(t *testing.T) {
	elems := []float64{0.1, 0.3, 0.6}
	band := ProbabilityElems(elems, 0.1)
	if len(band) != 3 {
		t.Errorf("ProbabilityElems() length = %v, want 3", len(band))
	}
}

func TestProbability(t *testing.T) {
	band := []int{1, 4, 10}
	index, _ := Probability(band)
	if index < 0 || index >= len(band) {
		t.Errorf("Probability() index = %v, want [0, %v)", index, len(band))
	}
}

func TestClamp_Float64(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		lo   float64
		hi   float64
		want float64
	}{
		{"in range", 5.0, 0.0, 10.0, 5.0},
		{"below lo", -1.0, 0.0, 10.0, 0.0},
		{"above hi", 15.0, 0.0, 10.0, 10.0},
		{"at lo boundary", 0.0, 0.0, 10.0, 0.0},
		{"at hi boundary", 10.0, 0.0, 10.0, 10.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clamp(tt.v, tt.lo, tt.hi); got != tt.want {
				t.Errorf("Clamp(%v, %v, %v) = %v, want %v", tt.v, tt.lo, tt.hi, got, tt.want)
			}
		})
	}
}

func TestClamp_Int(t *testing.T) {
	tests := []struct {
		name string
		v    int
		lo   int
		hi   int
		want int
	}{
		{"in range", 5, 1, 10, 5},
		{"below lo", 0, 1, 10, 1},
		{"above hi", 20, 1, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clamp(tt.v, tt.lo, tt.hi); got != tt.want {
				t.Errorf("Clamp(%v, %v, %v) = %v, want %v", tt.v, tt.lo, tt.hi, got, tt.want)
			}
		})
	}
}

func TestClamp_String(t *testing.T) {
	if got := Clamp("m", "a", "z"); got != "m" {
		t.Errorf("Clamp(\"m\", \"a\", \"z\") = %v, want m", got)
	}
	if got := Clamp("aaa", "b", "z"); got != "b" {
		t.Errorf("Clamp(\"aaa\", \"b\", \"z\") = %v, want b", got)
	}
	if got := Clamp("zzz", "a", "m"); got != "m" {
		t.Errorf("Clamp(\"zzz\", \"a\", \"m\") = %v, want m", got)
	}
}
