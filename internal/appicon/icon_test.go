package appicon

import "testing"

func TestRenderedIconHasTransparentCornersAndVisibleDocument(t *testing.T) {
    for _, size := range []int{16,32,48,256} {
        img:=Render(size)
        if img.Bounds().Dx()!=size || img.Bounds().Dy()!=size {
            t.Fatalf("bad dimensions for %d",size)
        }
        if img.NRGBAAt(0,0).A!=0 {
            t.Fatalf("%dpx icon needs transparent corners",size)
        }
        mid:=img.NRGBAAt(size/2,size/2)
        if mid.A < 200 {
            t.Fatalf("%dpx icon center should include visible paper",size)
        }
    }
}
