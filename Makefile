RELEASE:=$(shell git rev-parse --verify HEAD)

werf-clean:
	werf stages purge --stages-storage :local --force

werf-build:
	werf build --stages-storage :local

werf-publish: werf-build
	[ ! -z ${REGISTRY} ] || (echo "REGISRY not defined"; exit 1 );
	werf images publish --stages-storage :local --images-repo ${REGISTRY} --tag-git-commit=${RELEASE}

docker-build:
	docker build -f docker/Dockerfile --tag prometheus-exporter-gcp-quota:latest .

go-build:
	go build -o prometheus-exporter-gcp-quota .



