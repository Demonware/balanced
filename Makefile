VERSION            ?= $(shell git describe --tags --long)
ALIAS              ?= $(VERSION)
CMDS               := $(wildcard cmd/*)
DEPLOY_CONTAINERS  := $(wildcard deployment/containers/*)
DOCKER_REPOSITORY  ?= demonware
DOCKER_BASE        := $(DOCKER_REPOSITORY)
DOCKER_TEST_IMAGE  ?= $(DOCKER_BASE)/test-ipvs-controllers:$(VERSION)
DOCKER_IMAGE_FQ    = $(DOCKER_BASE)/$(basename $(*)):$(VERSION)
DOCKER_IMAGE_ALIAS ?= $(DOCKER_BASE)/$(basename $(*)):$(ALIAS)

.PHONY: images build clean test test-junit test-coverage-html test-image deployment-containers

images: docker deployment-containers

build: $(patsubst cmd/%, target/cmd/%, $(CMDS))

clean:
	rm -f target/*

target/cmd:
	mkdir -p $@

target/cmd/%: target/cmd
	CGO_ENABLED=0 GOOS=linux go build -tags netgo -o $@ cmd/$*/*.go

deployment-containers: $(patsubst deployment/containers/%, deployment-containers/%, $(DEPLOY_CONTAINERS))

deployment-containers/%:
	docker build -t $(DOCKER_IMAGE_FQ) deployment/containers/$*

docker: $(patsubst cmd/%, docker/%, $(CMDS))

docker/%:
	docker build --build-arg CMD=$(*) -t $(DOCKER_IMAGE_FQ) .

test:
	go test -v ./... -cover

test-junit:
	go test -v ./... -cover 2>&1 | go-junit-report > /tmp/report.xml

test-coverage-html:
	go test ./... -coverprofile=c.out || true
	go tool cover -html=c.out -o /tmp/coverage-index.html

coverage:
	mkdir coverage

test-image: coverage
	docker build -t ${DOCKER_TEST_IMAGE} -f Dockerfile.test .
	docker rm ipvs-test-results || true
	docker create --name ipvs-test-results ${DOCKER_TEST_IMAGE}
	docker cp ipvs-test-results:/tmp/report.xml coverage/test-report.xml
	docker cp ipvs-test-results:/tmp/coverage-index.html coverage/index.html
	docker rm ipvs-test-results
	docker rmi ${DOCKER_TEST_IMAGE}

push/%:
	docker push $(DOCKER_IMAGE_FQ)
