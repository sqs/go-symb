package foo

var NonLocalVar = 1

type NonLocalType int

func (localRecv *NonLocalType) NonLocalFunc(localParam int) (localResult int) {
	var localVar int = 1
	println(NonLocalVar, localRecv.NonLocalFunc, NonLocalType(3), localVar, localParam, localResult)
	return localVar
}
