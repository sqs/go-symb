.PHONY: test update-test-expectations

test: 
	GOPATH="$$PWD/test_gopath" go install foo bar
	go test go-symb

update-test-expectations:
	cd test_gopath/src/foo && \
	bash -c 'for src in *.go; do cp $${src}_actual.json $${src}_expected.json; done'
	cd test_gopath/src/bar && \
	bash -c 'for src in *.go; do cp $${src}_actual.json $${src}_expected.json; done'
