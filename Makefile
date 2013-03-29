.PHONY: update-test-expectations

update-test-expectations:
	cd test_gopath/src/foo && \
	bash -c 'for src in *.go; do cp $${src}_actual.json $${src}_expected.json; done'
