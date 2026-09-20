package appicon

import (
	"bytes"
	"image"
)

// imageEntry 是 ICO 中的一个图像条目。
type imageEntry struct {
	width  int
	height int
	// data 是传统位图（BITMAPINFOHEADER + 像素 + 掩码）或一份完整的 PNG 文件
	data []byte
}

// encodeICO 把图像条目打包成 ICO。
//
// 结构：ICONDIR + N 个 ICONDIRENTRY + 各条目数据。
func encodeICO(entries []imageEntry) []byte {
	const (
		dirSize   = 6
		entrySize = 16
	)

	var buf bytes.Buffer
	writeU16(&buf, 0)                    // 保留字段
	writeU16(&buf, 1)                    // 类型：1 = 图标
	writeU16(&buf, uint16(len(entries))) // 图像数量

	offset := dirSize + entrySize*len(entries)
	for _, e := range entries {
		buf.WriteByte(dimByte(e.width))  // 宽度（0 表示 256）
		buf.WriteByte(dimByte(e.height)) // 高度
		buf.WriteByte(0)                 // 调色板颜色数，真彩色为 0
		buf.WriteByte(0)                 // 保留字段
		writeU16(&buf, 1)                // 颜色平面数
		writeU16(&buf, 32)               // 每像素位数
		writeU32(&buf, uint32(len(e.data)))
		writeU32(&buf, uint32(offset)) // 图像数据偏移
		offset += len(e.data)
	}

	for _, e := range entries {
		buf.Write(e.data)
	}

	return buf.Bytes()
}

// dimByte 把边长写进 ICO 目录的单字节字段，256 记作 0。
func dimByte(v int) byte {
	if v >= 256 {
		return 0
	}
	return byte(v)
}

// encodeDIB 把图像编码成 ICO 使用的传统位图条目。
//
// 结构与独立 BMP 文件类似，但省去文件头、并多一段 AND 掩码：
// BITMAPINFOHEADER + 自下而上的 BGRA 像素 + 掩码。
func encodeDIB(img *image.NRGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	xorSize := w * h * 4            // 32bpp BGRA，每行天然 4 字节对齐
	andPitch := ((w + 31) / 32) * 4 // 掩码每像素 1 bit，每行按 4 字节对齐
	andSize := andPitch * h

	var buf bytes.Buffer

	// --- BITMAPINFOHEADER ---
	writeU32(&buf, 40)              // 结构体大小
	writeI32(&buf, int32(w))        // 宽度
	writeI32(&buf, int32(h*2))      // 高度：含掩码，故为实际高度的两倍
	writeU16(&buf, 1)               // 颜色平面数
	writeU16(&buf, 32)              // 每像素位数
	writeU32(&buf, 0)               // 压缩方式：0 = BI_RGB
	writeU32(&buf, uint32(xorSize)) // 像素数据大小
	writeU32(&buf, 0)               // 水平分辨率
	writeU32(&buf, 0)               // 垂直分辨率
	writeU32(&buf, 0)               // 调色板颜色数
	writeU32(&buf, 0)               // 重要颜色数

	// --- 像素数据：自下而上，BGRA 顺序 ---
	row := make([]byte, 0, w*4)
	for y := h - 1; y >= 0; y-- {
		row = row[:0]
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			row = append(row, c.B, c.G, c.R, c.A)
		}
		buf.Write(row)
	}

	// --- AND 掩码 ---
	// 32bpp 的透明度由 alpha 通道决定，掩码只需标出全透明像素，
	// 供不认 alpha 的老式渲染路径取用；但结构上必须存在。
	mask := make([]byte, andSize)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if img.NRGBAAt(b.Min.X+x, b.Min.Y+h-1-y).A == 0 {
				mask[y*andPitch+x/8] |= 0x80 >> (uint(x) % 8)
			}
		}
	}
	buf.Write(mask)

	return buf.Bytes()
}

func writeU16(buf *bytes.Buffer, v uint16) {
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
}

func writeU32(buf *bytes.Buffer, v uint32) {
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 24))
}

func writeI32(buf *bytes.Buffer, v int32) {
	writeU32(buf, uint32(v))
}
