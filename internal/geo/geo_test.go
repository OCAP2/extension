package geo

import (
	"errors"
	"testing"
)

func TestCoord3857FromString_ValidWithElevation(t *testing.T) {
	point, elev, err := Coord3857FromString("100.5,200.25,50.0")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X != 100.5 {
		t.Errorf("expected X=100.5, got %f", coords.X)
	}
	if coords.Y != 200.25 {
		t.Errorf("expected Y=200.25, got %f", coords.Y)
	}
	if elev != 50.0 {
		t.Errorf("expected elevation=50.0, got %f", elev)
	}
}

func TestCoord3857FromString_ValidWithoutElevation(t *testing.T) {
	point, elev, err := Coord3857FromString("100.5,200.25")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X != 100.5 {
		t.Errorf("expected X=100.5, got %f", coords.X)
	}
	if coords.Y != 200.25 {
		t.Errorf("expected Y=200.25, got %f", coords.Y)
	}
	if elev != 0 {
		t.Errorf("expected elevation=0, got %f", elev)
	}
}

func TestCoord3857FromString_NegativeCoordinates(t *testing.T) {
	point, elev, err := Coord3857FromString("-100.5,-200.25,-50.0")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X != -100.5 {
		t.Errorf("expected X=-100.5, got %f", coords.X)
	}
	if coords.Y != -200.25 {
		t.Errorf("expected Y=-200.25, got %f", coords.Y)
	}
	if elev != -50.0 {
		t.Errorf("expected elevation=-50.0, got %f", elev)
	}
}

func TestCoord3857FromString_IntegerCoordinates(t *testing.T) {
	point, _, err := Coord3857FromString("100,200")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X != 100 {
		t.Errorf("expected X=100, got %f", coords.X)
	}
	if coords.Y != 200 {
		t.Errorf("expected Y=200, got %f", coords.Y)
	}
}

func TestCoord3857FromString_ZeroCoordinates(t *testing.T) {
	point, elev, err := Coord3857FromString("0,0,0")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X != 0 {
		t.Errorf("expected X=0, got %f", coords.X)
	}
	if coords.Y != 0 {
		t.Errorf("expected Y=0, got %f", coords.Y)
	}
	if elev != 0 {
		t.Errorf("expected elevation=0, got %f", elev)
	}
}

func TestCoord3857FromString_InvalidTooFewComponents(t *testing.T) {
	_, _, err := Coord3857FromString("100.5")

	if err == nil {
		t.Fatal("expected error for invalid coordinates")
	}
	if !errors.Is(err, ErrInvalidCoordinates) {
		t.Errorf("expected ErrInvalidCoordinates, got %v", err)
	}
}

func TestCoord3857FromString_InvalidEmptyString(t *testing.T) {
	_, _, err := Coord3857FromString("")

	if err == nil {
		t.Fatal("expected error for empty string")
	}
	if !errors.Is(err, ErrInvalidCoordinates) {
		t.Errorf("expected ErrInvalidCoordinates, got %v", err)
	}
}

func TestCoord3857FromString_InvalidLongitude(t *testing.T) {
	_, _, err := Coord3857FromString("abc,200.25")

	if err == nil {
		t.Fatal("expected error for invalid longitude")
	}
	if !errors.Is(err, ErrInvalidCoordinates) {
		t.Errorf("expected ErrInvalidCoordinates, got %v", err)
	}
}

func TestCoord3857FromString_InvalidLatitude(t *testing.T) {
	_, _, err := Coord3857FromString("100.5,xyz")

	if err == nil {
		t.Fatal("expected error for invalid latitude")
	}
	if !errors.Is(err, ErrInvalidCoordinates) {
		t.Errorf("expected ErrInvalidCoordinates, got %v", err)
	}
}

func TestCoord3857FromString_InvalidElevation(t *testing.T) {
	_, _, err := Coord3857FromString("100.5,200.25,invalid")

	if err == nil {
		t.Fatal("expected error for invalid elevation")
	}
	if !errors.Is(err, ErrInvalidCoordinates) {
		t.Errorf("expected ErrInvalidCoordinates, got %v", err)
	}
}

func TestCoord3857FromString_ExtraComponents(t *testing.T) {
	// Extra components beyond 3 should be ignored
	point, elev, err := Coord3857FromString("100.5,200.25,50.0,extra,ignored")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X != 100.5 {
		t.Errorf("expected X=100.5, got %f", coords.X)
	}
	if coords.Y != 200.25 {
		t.Errorf("expected Y=200.25, got %f", coords.Y)
	}
	if elev != 50.0 {
		t.Errorf("expected elevation=50.0, got %f", elev)
	}
}

func TestCoord3857FromString_ScientificNotation(t *testing.T) {
	point, _, err := Coord3857FromString("1e2,2e3")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X != 100 {
		t.Errorf("expected X=100, got %f", coords.X)
	}
	if coords.Y != 2000 {
		t.Errorf("expected Y=2000, got %f", coords.Y)
	}
}

func TestCoord3857FromString_LargeCoordinates(t *testing.T) {
	point, _, err := Coord3857FromString("1000000.123456,2000000.654321")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X != 1000000.123456 {
		t.Errorf("expected X=1000000.123456, got %f", coords.X)
	}
	if coords.Y != 2000000.654321 {
		t.Errorf("expected Y=2000000.654321, got %f", coords.Y)
	}
}

func TestCoords3857From4326_ValidCoordinates(t *testing.T) {
	// Test converting WGS84 (EPSG:4326) to Web Mercator (EPSG:3857)
	// Approximate coordinates for a point
	point, err := Coords3857From4326(0, 0)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	// At (0, 0) in 4326, the 3857 coordinates should also be (0, 0)
	if coords.X != 0 {
		t.Errorf("expected X=0 at origin, got %f", coords.X)
	}
	if coords.Y != 0 {
		t.Errorf("expected Y=0 at origin, got %f", coords.Y)
	}
}

func TestCoords3857From4326_NonZeroCoordinates(t *testing.T) {
	// Test a point at 10 degrees longitude, 10 degrees latitude
	point, err := Coords3857From4326(10, 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	// In Web Mercator, these should be non-zero positive values
	if coords.X <= 0 {
		t.Errorf("expected positive X, got %f", coords.X)
	}
	if coords.Y <= 0 {
		t.Errorf("expected positive Y, got %f", coords.Y)
	}
}

func TestCoords3857From4326_NegativeCoordinates(t *testing.T) {
	// Test a point in the Southern/Western hemisphere
	point, err := Coords3857From4326(-45, -30)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	coords, ok := point.Coordinates()
	if !ok {
		t.Fatal("expected valid coordinates")
	}
	if coords.X >= 0 {
		t.Errorf("expected negative X for western hemisphere, got %f", coords.X)
	}
	if coords.Y >= 0 {
		t.Errorf("expected negative Y for southern hemisphere, got %f", coords.Y)
	}
}
