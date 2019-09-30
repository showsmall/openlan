package libol

import (
    "encoding/binary"
)

type Frame struct {
    Data []byte
}

func NewFrame(data []byte) (this *Frame) {
    this = &Frame{
        Data: make([]byte, len(data)),
    }
    copy(this.Data, data)
    return 
}

func (this *Frame) EthType() uint16 {
    return binary.BigEndian.Uint16(this.Data[12:14])
}

func (this *Frame) EthData() []byte {
    return this.Data[14:]
}

func (this *Frame) DstAddr() []byte {
    return this.Data[0:6]
}

func (this *Frame) SrcAddr() []byte {
    return this.Data[6:12]
}

func (this *Frame) EthParse() (uint16, []byte) {
    return this.EthType(), this.EthData()
}
