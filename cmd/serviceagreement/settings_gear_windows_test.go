//go:build windows

package main

import "testing"

func TestSettingsGearIsSmallTransparentIcon(t *testing.T) {
    img := makeSettingsGearImage()
    if img.Bounds().Dx() != 20 || img.Bounds().Dy() != 20 {
        t.Fatalf("settings icon must be compact: %v", img.Bounds())
    }
    // The transparent background and center hole guarantee this icon is
    // borderless rather than a drawn rectangular button.
    if img.NRGBAAt(0, 0).A != 0 || img.NRGBAAt(10, 10).A != 0 {
        t.Fatal("settings icon must have transparent corners and center")
    }
    filled := 0
    for y := 0; y < 20; y++ {
        for x := 0; x < 20; x++ {
            if img.NRGBAAt(x, y).A > 64 { filled++ }
        }
    }
    if filled < 100 || filled > 300 {
        t.Fatalf("unexpected gear silhouette size: %d pixels", filled)
    }
}
