//go:build windows

package main

import (
    "image"
    "image/color"
    "math"

    "github.com/lxn/walk"
)

// makeSettingsGearImage draws a small, antialiased, transparent gear silhouette.
// It is an image, not a Win32 PushButton: no border, background or button padding.
func makeSettingsGearImage() *image.NRGBA {
    const size = 20
    const samples = 4
    const teeth = 8

    img := image.NewNRGBA(image.Rect(0, 0, size, size))
    period := 2 * math.Pi / teeth
    for y := 0; y < size; y++ {
        for x := 0; x < size; x++ {
            covered := 0
            for iy := 0; iy < samples; iy++ {
                for ix := 0; ix < samples; ix++ {
                    dx := float64(x) + (float64(ix)+0.5)/samples - size/2.0
                    dy := float64(y) + (float64(iy)+0.5)/samples - size/2.0
                    radius := math.Hypot(dx, dy)
                    angle := math.Atan2(dy, dx)
                    angleFromTooth := math.Mod(angle+period/2+8*math.Pi, period) - period/2
                    outerRadius := 7.65
                    if math.Abs(angleFromTooth) < 0.22 {
                        outerRadius = 9.35
                    }
                    if radius >= 3.0 && radius <= outerRadius {
                        covered++
                    }
                }
            }
            if covered > 0 {
                img.SetNRGBA(x, y, color.NRGBA{
                    R: 67, G: 73, B: 81,
                    A: uint8(covered * 255 / (samples * samples)),
                })
            }
        }
    }
    return img
}

func makeSettingsGearBitmap() (*walk.Bitmap, error) {
    return walk.NewBitmapFromImage(makeSettingsGearImage())
}
