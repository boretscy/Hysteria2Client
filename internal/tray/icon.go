package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
)

// GenerateIconData создает растровую иконку 16x16.
// На Windows преобразует PNG в формат .ICO с правильным заголовком ICONDIR.
// Снабжает иконку 1-пиксельным контрастным белым контуром для отличной видимости на темной и светлой теме.
func GenerateIconData(innerColor color.RGBA) []byte {
	size := 16
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	center := 8
	radiusOuter := 6
	radiusInner := 5
	rOuterSq := radiusOuter * radiusOuter
	rInnerSq := radiusInner * radiusInner

	outlineColor := color.RGBA{R: 255, G: 255, B: 255, A: 240}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := x - center
			dy := y - center
			distSq := dx*dx + dy*dy
			if distSq <= rInnerSq {
				img.Set(x, y, innerColor)
			} else if distSq <= rOuterSq {
				img.Set(x, y, outlineColor)
			} else {
				img.Set(x, y, color.Transparent)
			}
		}
	}

	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	pngBytes := pngBuf.Bytes()

	if runtime.GOOS == "windows" {
		// Создаем стандартный одно-иконочный заголовок Windows .ICO
		var icoBuf bytes.Buffer
		// ICONDIR: Reserved (0), Type (1 = Icon), Count (1)
		binary.Write(&icoBuf, binary.LittleEndian, uint16(0))
		binary.Write(&icoBuf, binary.LittleEndian, uint16(1))
		binary.Write(&icoBuf, binary.LittleEndian, uint16(1))

		// ICONDIRENTRY: Width, Height, Colors, Reserved, Planes, BitCount, BytesInRes, ImageOffset
		icoBuf.WriteByte(byte(size))
		icoBuf.WriteByte(byte(size))
		icoBuf.WriteByte(0)
		icoBuf.WriteByte(0)
		binary.Write(&icoBuf, binary.LittleEndian, uint16(1))
		binary.Write(&icoBuf, binary.LittleEndian, uint16(32))
		binary.Write(&icoBuf, binary.LittleEndian, uint32(len(pngBytes)))
		binary.Write(&icoBuf, binary.LittleEndian, uint32(6+16)) // 6 (ICONDIR) + 16 (ICONDIRENTRY) = 22

		icoBuf.Write(pngBytes)
		return icoBuf.Bytes()
	}

	return pngBytes
}

// IconActive возвращает изумрудно-зеленый щит с белой окантовкой (туннель включен).
func IconActive() []byte {
	return GenerateIconData(color.RGBA{R: 46, G: 204, B: 113, A: 255})
}

// IconPaused возвращает нейтрально-серый щит с белой окантовкой (пауза / direct).
func IconPaused() []byte {
	return GenerateIconData(color.RGBA{R: 127, G: 140, B: 141, A: 255})
}

// IconDisconnected возвращает рубиново-красный щит с белой окантовкой (демон/служба недоступна).
func IconDisconnected() []byte {
	return GenerateIconData(color.RGBA{R: 231, G: 76, B: 60, A: 255})
}
