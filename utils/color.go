package utils

import "github.com/gookit/color"

func Black(a ...interface{}) string {
	return color.Black.Render(a...)
}

func White(a ...interface{}) string {
	return color.White.Render(a...)
}

func Gray(a ...interface{}) string {
	return color.Gray.Render(a...)
}

func Red(a ...interface{}) string {
	return color.Red.Render(a...)
}

func Green(a ...interface{}) string {
	return color.Green.Render(a...)
}

func Yellow(a ...interface{}) string {
	return color.Yellow.Render(a...)
}

func Blue(a ...interface{}) string {
	return color.Blue.Render(a...)
}

func Magenta(a ...interface{}) string {
	return color.Magenta.Render(a...)
}

func Cyan(a ...interface{}) string {
	return color.Cyan.Render(a...)
}
