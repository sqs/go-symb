.PHONY: test update-test-expectations

test: 
	GOPATH="$$PWD/testdata" go install foo bar
	go test go-symb

update-test-expectations:
	cd testdata/src/foo && \
	bash -c 'for src in *.go; do cp $${src}_actual.json $${src}_expected.json; done'
	cd testdata/src/bar && \
	bash -c 'for src in *.go; do cp $${src}_actual.json $${src}_expected.json; done'
