package foo

type R int

func (r R) M() {}

func init() {
	R(7).M()
}
