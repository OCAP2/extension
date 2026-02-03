package geo

import (
	"testing"
)

func TestParsePolyline_Valid(t *testing.T) {
	input := "[[100.5,200.25],[300.75,400.5],[500,600]]"

	ls, err := ParsePolyline(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	seq := ls.Coordinates()
	if seq.Length() != 3 {
		t.Fatalf("expected 3 points, got %d", seq.Length())
	}

	expected := [][2]float64{
		{100.5, 200.25},
		{300.75, 400.5},
		{500, 600},
	}
	for i := 0; i < seq.Length(); i++ {
		pt := seq.GetXY(i)
		if pt.X != expected[i][0] || pt.Y != expected[i][1] {
			t.Errorf("point %d: expected (%f,%f), got (%f,%f)", i, expected[i][0], expected[i][1], pt.X, pt.Y)
		}
	}
}

func TestParsePolyline_TwoPoints(t *testing.T) {
	input := "[[0,0],[100,100]]"

	ls, err := ParsePolyline(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ls.Coordinates().Length() != 2 {
		t.Fatalf("expected 2 points, got %d", ls.Coordinates().Length())
	}
}

func TestParsePolyline_InvalidJSON(t *testing.T) {
	input := "not valid json"

	_, err := ParsePolyline(input)

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParsePolyline_TooFewPoints(t *testing.T) {
	input := "[[100,200]]"

	_, err := ParsePolyline(input)

	if err == nil {
		t.Fatal("expected error for single point")
	}
}

func TestParsePolyline_EmptyArray(t *testing.T) {
	input := "[]"

	_, err := ParsePolyline(input)

	if err == nil {
		t.Fatal("expected error for empty array")
	}
}

func TestParsePolyline_InsufficientCoordinates(t *testing.T) {
	input := "[[100],[200,300]]"

	_, err := ParsePolyline(input)

	if err == nil {
		t.Fatal("expected error for coordinate with single value")
	}
}
