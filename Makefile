build:
	docker run -w /app -v ${PWD}:/app --rm golang go build -v -a -tags config-check-extension \
		-o release/linux/amd64/aws-config-check-extension ./cmd/aws-config-check-extension

build-container:
	docker build -t strithoncloud/aws-config-check-extension .

develop:
	docker run -it --rm -v ${PWD}:/app strithoncloud/aws-config-check-extension

run:
	go build -o release/linux/amd64 plugin

gitea: build-container
	docker-compose up -d

build-test:
	docker build -t strithoncloud/aws-config-check-extension:test --target test .

e2e-test:
	python -m pytest -vv tests/e2e -W ignore::DeprecationWarning

test:
	docker run --rm -v ${PWD}:/app strithoncloud/aws-config-check-extension:test go test -v plugin/*.go

cov:
	docker run --rm -v ${PWD}:/app strithoncloud/aws-config-check-extension:test \
	  go test -coverprofile=coverage.out plugin/*.go && cat coverage.out

.PHONY: gitea
