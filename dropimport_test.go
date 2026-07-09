package main

import (
	"strings"
	"testing"
)

// TestSceneFromFile checks the drag-and-drop dispatch: an .svg becomes an
// imported scene, an .afoil loads as a scene file, anything else errors.
func TestSceneFromFile(t *testing.T) {
	svg := []byte(`<svg><path d="M0 0 H10 V5 H0 Z"/></svg>`)
	sc, err := sceneFromFile("car.svg", svg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Objects) != 1 || sc.Objects[0].Name != "car" {
		t.Fatalf("svg drop: got %+v", sc.Objects)
	}

	afoil := []byte(`(scene (object "wing" (naca "2412" 130 90 100)) (loop 0))`)
	sc, err = sceneFromFile("wing.afoil", afoil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Objects) != 1 || sc.Objects[0].Name != "wing" {
		t.Fatalf("afoil drop: got %+v", sc.Objects)
	}
}

// TestSceneFromFileCaseInsensitive checks extensions match regardless of case,
// which file managers on Windows produce freely.
func TestSceneFromFileCaseInsensitive(t *testing.T) {
	svg := []byte(`<svg><path d="M0 0 H10 V5 H0 Z"/></svg>`)
	_, err := sceneFromFile("CAR.SVG", svg)
	if err != nil {
		t.Fatalf("uppercase extension: %v", err)
	}
}

// TestSceneFromFileErrors checks unknown extensions and broken content fail
// with an error naming the problem, since the message lands in the UI.
func TestSceneFromFileErrors(t *testing.T) {
	_, err := sceneFromFile("photo.png", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), ".png") {
		t.Errorf("unknown extension: got %v", err)
	}
	_, err = sceneFromFile("bad.svg", []byte("not xml at all"))
	if err == nil {
		t.Error("expected an error for a broken svg")
	}
	_, err = sceneFromFile("bad.afoil", []byte("(nonsense"))
	if err == nil {
		t.Error("expected an error for a broken afoil")
	}
}
