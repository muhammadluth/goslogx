.PHONY: test-all check-core check-layout bench

test-all:
	bash scripts/test-modules.sh

check-core:
	bash scripts/check-core-deps.sh

check-layout:
	bash scripts/check-plugin-layout.sh

bench:
	go test -run='^$$' -bench=. -benchmem ./...
