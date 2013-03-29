package foo

func B() {
	eB, fB, _ := A("a", "b", true)
	eB = fB
	fB = eB
}
