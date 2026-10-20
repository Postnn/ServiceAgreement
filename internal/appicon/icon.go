package appicon

import (
    "image"
    "image/color"
    "math"
)

// Render returns a transparent document-and-checkmark application icon.
// Vector geometry is supersampled so that 16px title-bar variants remain crisp.
func Render(size int) *image.NRGBA {
    if size < 1 {
        size = 32
    }
    const sub = 4
    img := image.NewNRGBA(image.Rect(0, 0, size, size))
    for y := 0; y < size; y++ {
        for x := 0; x < size; x++ {
            var rr, gg, bb, aa float64
            for sy := 0; sy < sub; sy++ {
                for sx := 0; sx < sub; sx++ {
                    px := (float64(x)+(float64(sx)+0.5)/sub)*96/float64(size)
                    py := (float64(y)+(float64(sy)+0.5)/sub)*96/float64(size)
                    c := at(px, py)
                    a := float64(c.A) / 255
                    rr += float64(c.R)*a
                    gg += float64(c.G)*a
                    bb += float64(c.B)*a
                    aa += a
                }
            }
            if aa == 0 {
                continue
            }
            inv := 1/(float64(sub*sub))
            img.SetNRGBA(x,y,color.NRGBA{
                R:uint8(math.Round(rr/aa)),G:uint8(math.Round(gg/aa)),
                B:uint8(math.Round(bb/aa)),A:uint8(math.Round(aa*inv*255)),
            })
        }
    }
    return img
}

func rounded(x,y,l,t,r,b,rad float64) bool {
    if x<l || x>r || y<t || y>b {return false}
    cx:=math.Max(l+rad,math.Min(r-rad,x))
    cy:=math.Max(t+rad,math.Min(b-rad,y))
    return math.Hypot(x-cx,y-cy)<=rad
}

func segment(x,y,x1,y1,x2,y2,width float64) bool {
    dx,dy:=x2-x1,y2-y1
    t:=math.Max(0,math.Min(1,((x-x1)*dx+(y-y1)*dy)/(dx*dx+dy*dy)))
    return math.Hypot(x-(x1+t*dx),y-(y1+t*dy))<=width/2
}

func at(x,y float64) color.NRGBA {
    clear:=color.NRGBA{}
    if !rounded(x,y,5,5,91,91,21) {return clear}
    // Deep-blue square: a stable, recognizable app/taskbar silhouette.
    c:=color.NRGBA{R:27,G:66,B:111,A:255}
    // White paper with upper-right folded corner.
    if rounded(x,y,25,15,71,77,5) {
        c=color.NRGBA{R:248,G:251,B:255,A:255}
    }
    if x>59 && x<71 && y>=15 && y<27 && x+y<86 {
        c=color.NRGBA{R:199,G:215,B:233,A:255}
    }
    // Three text lines, like clauses in a service agreement.
    for _, cy:=range []float64{35,44,53} {
        if rounded(x,y,33,cy,63,cy+3.7,1.7) {
            c=color.NRGBA{R:101,G:134,B:169,A:255}
        }
    }
    // Checkmark for approved contract/service terms.
    if segment(x,y,38,66,47,75,8) || segment(x,y,47,75,65,57,8) {
        c=color.NRGBA{R:18,G:176,B:126,A:255}
    }
    return c
}
