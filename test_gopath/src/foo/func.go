package foo

func A(b, c string, d bool) (e, f int, g uint) {
	bb := b
	d = true
	e = 7
	f = len(c + bb)
	g = uint(e)
	return
}
