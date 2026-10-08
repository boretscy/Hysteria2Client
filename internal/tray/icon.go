package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// GenerateIconData создает простую растровую иконку 16x16 PNG заданного цвета.
func GenerateIconData(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	
	// Рисуем кружок по центру
	center := 8
	radius := 6
	radiusSq := radius * radius

	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			dx := x - center
			dy := y - center
			if dx*dx+dy*dy <= radiusSq {
				img.Set(x, y, c)
			} else {
				img.Set(x, y, color.Transparent)
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// IconActive возвращает темную/черную иконку (туннель включен).
func IconActive() []byte {
	return GenerateIconData(color.RGBA{R: 30, G: 30, B: 30, A: 255})
}

// IconPaused возвращает серую иконку (пауза / direct).
func IconPaused() []byte {
	return GenerateIconData(color.RGBA{R: 149, G: 165, B: 166, A: 255})
}

// IconDisconnected возвращает красную иконку (демон недоступен).
func IconDisconnected() []byte {
	return GenerateIconData(color.RGBA{R: 231, G: 76, B: 60, A: 255})
}
