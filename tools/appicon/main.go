// A tiny standard-library-only icon builder for the Windows resource step.
// Produces multi-size PNG-compressed ICO (16/32/48/256 px).
package main

import (
    "bytes"
    "encoding/binary"
    "fmt"
    "image/png"
    "os"
    "path/filepath"

    "github.com/Postnn/ServiceAgreement/internal/appicon"
)

func main() {
    if len(os.Args)!=2 {panic("usage: go run ./tools/appicon path/to/icon.ico")}
    sizes:=[]int{16,32,48,256}
    payloads:=make([][]byte,len(sizes))
    for i,sz:=range sizes {
        var b bytes.Buffer
        if err:=png.Encode(&b,appicon.Render(sz));err!=nil {panic(err)}
        payloads[i]=b.Bytes()
    }
    var ico bytes.Buffer
    for _,v:=range []uint16{0,1,uint16(len(sizes))} {
        if err:=binary.Write(&ico,binary.LittleEndian,v);err!=nil {panic(err)}
    }
    off:=uint32(6+16*len(sizes))
    for i,sz:=range sizes {
        width:=byte(sz)
        if sz==256 {width=0}
        ico.Write([]byte{width,width,0,0})
        for _,v:=range []uint16{1,32} {
            if err:=binary.Write(&ico,binary.LittleEndian,v);err!=nil {panic(err)}
        }
        for _,v:=range []uint32{uint32(len(payloads[i])),off} {
            if err:=binary.Write(&ico,binary.LittleEndian,v);err!=nil {panic(err)}
        }
        off+=uint32(len(payloads[i]))
    }
    for _,data:=range payloads {ico.Write(data)}
    dest:=os.Args[1]
    if err:=os.MkdirAll(filepath.Dir(dest),0755);err!=nil {panic(err)}
    if err:=os.WriteFile(dest,ico.Bytes(),0644);err!=nil {panic(err)}
    fmt.Printf("generated %s (%d bytes)\n",dest,ico.Len())
}
