.PHONY: build test helm-lint helm-dep-update e2e e2e-setup e2e-teardown

build:
	cd replay-manager && go build -o ../bin/replay-manager .

test:
	cd replay-manager && go test ./...

helm-lint:
	helm lint charts/prom-replay

helm-dep-update:
	helm dependency update charts/prom-replay

e2e:
	./e2e/run.sh

e2e-setup:
	SKIP_TEARDOWN=true ./e2e/run.sh

e2e-teardown:
	kind delete cluster --name prom-replay-e2e
