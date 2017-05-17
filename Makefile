.PHONY: help scratch run test rpm docker docker-scratch with-docker docker-setup docker-baseimage docker-build docker-rpmtest docker-clean clean

gracc-collector: *.go
	go build -ldflags "-X main.build_date=`date -u +%Y%m%d.%H%M%S` -X main.build_ref=RELEASE" -o gracc-collector

help:
	 @echo  'Build Targets'
	 @echo  '  scratch         - build scratch executable with race checking'
	 @echo  '  run             - build temporary exectutable and run it'
	 @echo  '  test            - build and run tests via "go test"'
	 @echo  '  rpm             - build RPM from HEAD'
	 @echo  '  docker          - build deployable centos-based docker image'
	 @echo  '  docker-scratch  - build minimal deployable docker image'
	 @echo  '  with-docker     - build & test in docker image.'
	 @echo  '                    if successful, saves exectuable and RPM in targets/'
	 @echo  ''
	 @echo  'Clean Targets'
	 @echo  '  docker-clean    - cleans up docker images and networks from with-docker'
	 @echo  '  clean           - cleans up executables'
	 @echo  ''

scratch: *.go
	go build -ldflags "-X main.build_date=`date -u +%Y%m%d.%H%M%S` -X main.build_ref=`git rev-parse --verify HEAD --short`" -race -o gracc-collector.scratch

run:
	go build -ldflags "-X main.build_date=`date -u +%Y%m%d.%H%M%S`" -o gracc.run -race && ./gracc.run; rm -f gracc.run

test:
	# test gracc library
	cd gracc; go test
	# test main
	go test

rpm:
	git archive --prefix gracc-collector/ --output $(HOME)/rpmbuild/SOURCES/gracc-collector.tar.gz HEAD
	rpmbuild -ba gracc-collector.spec

docker:
	docker build -t opensciencegrid/gracc-collector .

docker-scratch:
	CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags "-X main.build_date=`date -u +%Y%m%d.%H%M%S` -X main.build_ref=RELEASE -w" -o gracc-collector
	docker build -t opensciencegrid/gracc-collector:scratch -f Dockerfile.scratch .

with-docker: | docker-setup docker-build docker-rpmtest docker-clean

docker-setup:
	@echo
	@echo '################################################################################'
	@echo '### Setting up build & test environment                                      ###'
	@echo '################################################################################'
	docker network create graccbuild
	docker run -d --network graccbuild --name graccbuild-rabbit rabbitmq
	docker run -d --network graccbuild --name graccbuild-stash \
		-e 'GRACC_STREAM=test' -e 'GRACC_INSTANCE=test' \
		-e 'AMQP_HOST=graccbuild-rabbit' \
		retzkek/gracc-stash-raw:debug

docker-baseimage:
	docker build -f Dockerfile.golang.centos7 -t local/golang:centos7 .

docker-build: docker-baseimage
	@echo
	@echo '################################################################################'
	@echo '### Building executable and RPM                                              ###'
	@echo '################################################################################'
	docker build -f Dockerfile.build --no-cache -t opensciencegrid/gracc-collector-test .
	docker run -it --network graccbuild --name graccbuild -e GRACC_AMQP_HOST=graccbuild-rabbit opensciencegrid/gracc-collector-test
	mkdir -p target
	docker cp graccbuild:/root/rpmbuild/RPMS/ ./target/
	docker cp graccbuild:/root/rpmbuild/BUILD/gracc-collector/gracc-collector ./target/

docker-rpmtest:
	@echo
	@echo '################################################################################'
	@echo '### Performing integration tests                                             ###'
	@echo '################################################################################'
	docker build --no-cache -f Dockerfile.rpmtest -t opensciencegrid/gracc-rpmtest .
	docker run --privileged -d --network graccbuild --name graccbuild-rpmtest -v /sys/fs/cgroup:/sys/fs/cgroup:ro -p 8080:8080 opensciencegrid/gracc-rpmtest
	sleep 5 # let systemd start up
	-docker exec graccbuild-rpmtest /usr/bin/systemctl status gracc-collector
	-curl -XPOST -i 'localhost:8080/gratia-servlets/rmi?command=update&from=localhost&bundlesize=15' --data-urlencode arg1@test.bundle
	sleep 5 # let records get through rabbitmq and logstash
	-docker exec graccbuild-rpmtest cat /var/log/gracc/gracc-collector.log
	-docker logs graccbuild-stash
	docker stop graccbuild-rpmtest

docker-clean:
	-docker rm -f graccbuild-rabbit graccbuild-stash graccbuild graccbuild-rpmtest
	-docker network rm graccbuild

clean:
	rm -f gracc-collector gracc-collector.scratch gracc.run
